package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	const maxMemory = 10 << 20
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

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// TODO: implement the upload here
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not parse submission", err)
		return
	}

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error retrieving file", err)
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	var fileExtension string

	switch contentType {
	case "text/html":
		fileExtension = ".html"
	case "image/jpeg":
		fileExtension = ".jpeg"
	case "image/png":
		fileExtension = ".png"
	case "application/pdf":
		fileExtension = ".pdf"
	case "application/json":
		fileExtension = ".json"
	case "video/mp4":
		fileExtension = ".mp4"
	case "audio/mp3":
		fileExtension = ".mp3"
	default:
		fileExtension = ".txt"
	}

	filePath := filepath.Join(cfg.assetsRoot, videoIDString+fileExtension)

	newFile, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create file", err)
		return
	}
	defer newFile.Close()
	/*
		fileBytes, err := io.ReadAll(file)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Error reading file into buffer", err)
			return
		}
	*/
	if _, err := io.Copy(newFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not write to file", err)
		return
	}

	//	file64 := base64.StdEncoding.EncodeToString(fileBytes)

	videoData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error retrieving video metadata", err)
		return
	}
	if videoData.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "User is not video owner", err)
		return
	}
	/*	thumbnail := thumbnail{
			data:      fileBytes,
			mediaType: contentType,
		}
	*/

	videoURL := fmt.Sprintf("http://localhost:%s/assets/%s%s", cfg.port, videoIDString, fileExtension)
	videoData.ThumbnailURL = &videoURL

	err = cfg.db.UpdateVideo(videoData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not update video metadata", err)
		return
	}

	//videoThumbnails[videoID] = thumbnail

	respondWithJSON(w, http.StatusOK, videoData)
}
