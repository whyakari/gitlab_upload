package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"log"
	// "github.com/joho/godotenv"
)

var (
	gitlabToken string
	projectID   string
)

type zipInfo struct {
	path string
	ts   int64
}

func init() {

	gitlabToken = os.Getenv("GITLAB_TOKEN")
	projectID = os.Getenv("PROJECTID_GITLAB")

	if gitlabToken == "" || projectID == "" {
		log.Fatal("GITLAB_TOKEN or PROJECTID_GITLAB not defined in .env")
	}
}

func fileAlreadyExists(tagName, fileName string) (bool, error) {
	url := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/packages?package_name=builds&package_version=%s", 
		projectID, tagName)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("PRIVATE-TOKEN", gitlabToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var packages []struct {
		ID int `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&packages)

	if len(packages) == 0 {
		return false, nil
	}

	filesUrl := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/packages/%d/package_files", projectID, packages[0].ID)
	reqFile, _ := http.NewRequest("GET", filesUrl, nil)
	reqFile.Header.Set("PRIVATE-TOKEN", gitlabToken)
	
	respFile, _ := client.Do(reqFile)
	defer respFile.Body.Close()

	var files []struct {
		FileName string `json:"file_name"`
	}
	json.NewDecoder(respFile.Body).Decode(&files)

	for _, f := range files {
		if f.FileName == fileName {
			return true, nil
		}
	}
	return false, nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: ./lab <device>")
		os.Exit(1)
	}

	device := os.Args[1]
	baseDir := fmt.Sprintf("out/target/product/%s", device)

	zipPattern := filepath.Join(baseDir, "*.zip")
	zipFiles, _ := filepath.Glob(zipPattern)
	
	var parsed []zipInfo
	for _, z := range zipFiles {
		base := filepath.Base(z)
		if strings.HasSuffix(strings.ToLower(base), "-ota.zip") {
			continue
		}

		parts := strings.Split(base, "-")
		if len(parts) < 3 {
			continue
		}

		datePart := parts[len(parts)-2]
		timePart := strings.TrimSuffix(parts[len(parts)-1], ".zip")

		tsStr := datePart + timePart
		var ts int64
		fmt.Sscanf(tsStr, "%d", &ts)

		parsed = append(parsed, zipInfo{path: z, ts: ts})
	}

	if len(parsed) == 0 {
		fmt.Println("❌ No valid ZIP found.")
		os.Exit(1)
	}

	sort.Slice(parsed, func(i, j int) bool {
		return parsed[i].ts > parsed[j].ts
	})

	latestZip := parsed[0].path
	zipName := filepath.Base(latestZip)
	tagName := time.Now().Format("20060102")

	fmt.Printf("📦 Target ZIP: %s (TS: %d)\n", zipName, parsed[0].ts)

	exists, _ := fileAlreadyExists(tagName, zipName)
	if exists {
		fmt.Printf("⏭ Build %s already exists on GitLab. Skipping upload.\n", zipName)
		fmt.Printf("OTA_URL_RESULT: https://gitlab.com/api/v4/projects/%s/packages/generic/builds/%s/%s\n", 
			projectID, tagName, zipName)
		os.Exit(0)
	}

	filesToUpload := []string{latestZip}
	client := &http.Client{}
	for _, f := range filesToUpload {
		currName := filepath.Base(f)
		fmt.Printf("📤 Uploading: %s...\n", currName)
		
		file, _ := os.Open(f)
		uploadURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/packages/generic/builds/%s/%s", 
			projectID, tagName, currName)

		req, _ := http.NewRequest("PUT", uploadURL, file)
		req.Header.Set("PRIVATE-TOKEN", gitlabToken)

		resp, err := client.Do(req)
		if err == nil && (resp.StatusCode == 201 || resp.StatusCode == 200) {
			fmt.Printf("✅ %s uploaded.\n", currName)
			if strings.HasSuffix(currName, ".zip") {
				fmt.Printf("OTA_URL_RESULT: %s\n", uploadURL)
			}
			resp.Body.Close()
		} else {
			fmt.Printf("❌ Failed to upload %s\n", currName)
		}
		file.Close()
	}
}
