package main

import (
	"fmt"
	"net/http"
	"io"
	"os"
	"path/filepath"
	"mime"
	"encoding/base64"
	"crypto/rand"

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

	media_type, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to get media-type", err)
		return 
	}

	if media_type != "image/jpeg" && media_type != "image/png" {
		respondWithError(w, http.StatusBadRequest, "cannot upload a non-image file as thumbnail", err)
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
	


	extensions, err := mime.ExtensionsByType(media_type) 

	if err != nil || len(extensions) == 0 {
		respondWithError(w, http.StatusInternalServerError, "unable to get file extension", err)
		return 
	}

	extension := extensions[0]
	key := make([]byte, 32)
	rand.Read(key)

	raw_URL := base64.RawURLEncoding.EncodeToString(key)
	file_name := raw_URL + extension

	final_path := filepath.Join(cfg.assetsRoot, file_name)

	final_file, err := os.Create(final_path)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "cannot create/modify file at the given path", err)
		return 
	}

	_, err = io.Copy(final_file, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to copy file from source to destination", err)
		return 
	}


	thumbnailURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, file_name)

	video.ThumbnailURL = &thumbnailURL

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to update the video", err)
		return 
	}

	respondWithJSON(w, http.StatusOK, video)
}
