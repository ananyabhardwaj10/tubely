package main

import (
	"net/http"
	"mime"
	"os"
	"io"
	"encoding/json"
	"os/exec"
	"bytes"
	"math"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1 << 30) 
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
		respondWithError(w, http.StatusUnauthorized, "Unable to validate JWT", err)
		return 
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error extracting the video from database", err)
		return 
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized access", nil)
		return 
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return 
	}

	defer file.Close()

	media_type, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to get media-type", err)
		return 
	}

	if media_type != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Only mp4 videos allowed", err)
		return 
	}

	temp_file,  err := os.CreateTemp("", "tubely-upload.mp4") 
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to save the file temporarily on disk", err)
		return 
	}

	defer os.Remove(temp_file.Name())
	defer temp_file.Close()

	_, err = io.Copy(temp_file, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error copying the file", err)
		return 
	}
	
	_, err = temp_file.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to reset the file pointer", err)
		return
	}

	aspectRatio, err := getVideoAspectRatio(temp_file.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error getting the aspect ratio", err)
		return 
	}

	key := getAssetPath(media_type)

	var final_key, prefix string 

	if aspectRatio == "16:9" {
		prefix = "landscape"
	} else if aspectRatio == "9:16" {
		prefix = "portrait"
	} else {
		prefix = "other"
	}

	final_key = prefix + "/" +  key

	processed_file_path, err := processVideoForFastStart(temp_file.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to process the video for a fast start", err)
		return
	}

	defer os.Remove(processed_file_path)

	processed_file, err := os.Open(processed_file_path)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error opening the processed file", err)
		return 
	}


	defer processed_file.Close()

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket: aws.String(cfg.s3Bucket), 
		Key: aws.String(final_key),
		Body: processed_file,
		ContentType: aws.String(media_type),
	}) 

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to put object in aws s3", err)
		return 
	}

	url := fmt.Sprintf("https://%s/%s",cfg.s3CfDistribution, final_key)

	video.VideoURL = &url 

	err = cfg.db.UpdateVideo(video) 
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error updating video", err)
		return 
	}

	respondWithJSON(w, http.StatusOK, video)

}

func getVideoAspectRatio(filePath string) (string, error) {
	var buffer bytes.Buffer 
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	cmd.Stdout = &buffer
	err := cmd.Run()
	if err != nil {
		return "", err  
	}

	var result struct {
		Streams []struct {
			Width int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}

	err = json.Unmarshal(buffer.Bytes(), &result)
	if err != nil {
		return "", err
	}

	ratio := float64(result.Streams[0].Width) / float64(result.Streams[0].Height)

	if math.Abs(ratio - 16.0/9.0) < 0.01 {
    	return "16:9", nil
	}

	if math.Abs(ratio - 9.0/16.0) < 0.01 {
    	return "9:16", nil
	}	

	return "other", nil

}

func processVideoForFastStart(filePath string) (string, error) {
	output_path := filePath + ".processing"

	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", output_path)
	err := cmd.Run()
	if err != nil {
		return "", err 
	}

	return output_path, nil
}

