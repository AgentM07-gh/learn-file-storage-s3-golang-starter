package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const maxMemory = 10 << 30

	defer r.Body.Close()

	r.Body = http.MaxBytesReader(w, r.Body, maxMemory)

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	videoMetaData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error retrieving video", err)
		return
	}
	if videoMetaData.UserID != userID {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error retrieving file", err)
		return
	}
	defer file.Close()

	contentType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil || (contentType != "video/mp4") {
		respondWithError(w, http.StatusInternalServerError, "Invalid Content-Type", err)
		return
	}

	videoFile, err := os.CreateTemp("", "tubely-upload.mp4")

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error retrieving file", err)
		return
	}

	defer os.Remove(videoFile.Name())
	defer videoFile.Close()

	_, err = io.Copy(videoFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not copy file", err)
		return
	}

	_, err = videoFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not set file pointer", err)
		return
	}

	aspectRatio, err := getVideoAspectRatio(videoFile.Name())

	var aspectRatioString string

	if aspectRatio == "16:9" {
		aspectRatioString = "landscape/"
	} else if aspectRatio == "9:16" {
		aspectRatioString = "portrait/"
	} else {
		aspectRatioString = "other/"
	}

	name := make([]byte, 32)
	rand.Read(name)
	fileName := aspectRatioString + hex.EncodeToString(name) + ".mp4"

	params := &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &fileName,
		Body:        videoFile,
		ContentType: &contentType,
	}

	_, err = cfg.s3Client.PutObject(r.Context(), params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error uploading file", err)
		return
	}

	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, fileName)

	videoMetaData.VideoURL = &videoURL

	err = cfg.db.UpdateVideo(videoMetaData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not update video metadata", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoMetaData)
}

func getVideoAspectRatio(filePath string) (string, error) {

	var outBuff bytes.Buffer

	videoData := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	videoData.Stdout = &outBuff

	err := videoData.Run()
	if err != nil {
		return "", err
	}

	type Stream struct {
		CodecType string `json:"codec_type"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	}

	type ProbeResult struct {
		Streams []Stream `json:"streams"`
	}

	var results ProbeResult
	err = json.Unmarshal(outBuff.Bytes(), &results)
	if err != nil {
		return "", err
	}

	height := 0
	width := 0

	for _, result := range results.Streams {
		if result.CodecType == "video" {
			height = result.Height
			width = result.Width
		}
	}
	if height == 0 || width == 0 {
		return "", errors.New("No valid videos found")
	}

	ratio := float64(width) / float64(height)

	landscapeTarget := 16.0 / 9.0
	portraitTarget := 9.0 / 16.0
	epsilon := 0.01

	if math.Abs(ratio-landscapeTarget) < epsilon {
		return "16:9", nil
	} else if math.Abs(ratio-portraitTarget) < epsilon {
		return "9:16", nil
	} else {
		return "other", nil
	}

	/*
	   gcd := gcdUsingEuclidean(width, height)
	   aw := width / gcd
	   ah := height / gcd

	   	if aw == 16 && ah == 9 {
	   		return "16:9", nil
	   	} else if aw == 9 && ah == 16 {

	   		return "9:16", nil
	   	} else {

	   		return "other", nil
	   	}
	*/
}

/*
func gcdUsingEuclidean(a, b int) int {
	for b != 0 {
		// This line simultaneously updates a to b, and b to the remainder of a / b
		a, b = b, a%b
	}
	return a // When b is 0, a holds the GCF
}
*/
