package main

import (
	"fmt"
	"net/http"
	"io"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
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

	const maxMemory = 10 << 20 

	r.ParseMultipartForm(maxMemory)

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "unable to parse form file", err)
		return 
	}

	defer file.Close()

	media_type := header.Header.Get("Content-Type")

	var data []byte

	data, err = io.ReadAll(file)   
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to read data from the file", err)
		return 
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error extracting the video from database", err)
		return 
	}

	if video.UserID != userID {
			respondWithError(w, http.StatusUnauthorized, "Unauthorized access", err)
			return 
		}

	videoThumbnails[videoID] = thumbnail{
		data: data,
		mediaType: media_type,
	}

	thumbnailURL := fmt.Sprintf("http://localhost%v/api/thumbnails/%v", cfg.port, videoID)

	video.ThumbnailURL = &thumbnailURL

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to update the video", err)
		delete(videoThumbnails, videoID)
		return 
	}

	respondWithJSON(w, http.StatusOK, video)
}
