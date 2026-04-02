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
)

var (
	gitlabToken = os.Getenv("GITLAB_TOKEN")
	projectID   = "ruri%2Fayaka-releases"
)

// Estrutura para ler a resposta da API do GitLab
type GitLabPackageFile struct {
	FileName string `json:"file_name"`
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

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	var packages []struct {
		ID int `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&packages)

	if len(packages) == 0 {
		return false, nil
	}

	filesUrl := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/packages/%d/package_files", 
		projectID, packages[0].ID)
	
	reqFile, _ := http.NewRequest("GET", filesUrl, nil)
	reqFile.Header.Set("PRIVATE-TOKEN", gitlabToken)
	
	respFile, err := client.Do(reqFile)
	if err != nil {
		return false, err
	}
	defer respFile.Body.Close()

	var files []GitLabPackageFile
	json.NewDecoder(respFile.Body).Decode(&files)

	for _, f := range files {
		if f.FileName == fileName {
			return true, nil
		}
	}

	return false, nil
}

func gitlabUpload(filePath string) error {
	fileName := filepath.Base(filePath)
	tagName := time.Now().Format("20060102")

	exists, err := fileAlreadyExists(tagName, fileName)
	if err != nil {
		fmt.Printf("! Erro ao verificar existência: %v\n", err)
	}
	if exists {
		fmt.Printf("⏭ Skipped: %s already exists on GitLab for version %s\n", fileName, tagName)
		
		if strings.HasSuffix(fileName, ".zip") {
			directURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/packages/generic/builds/%s/%s", 
				projectID, tagName, fileName)
			fmt.Printf("OTA_URL_RESULT: %s\n", directURL)
		}
		return nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	uploadURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/packages/generic/builds/%s/%s", 
		projectID, tagName, fileName)
	
	directURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/packages/generic/builds/%s/%s", 
		projectID, tagName, fileName)

	req, err := http.NewRequest("PUT", uploadURL, file)
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", gitlabToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		fmt.Printf("\n✓ Success: %s\n", fileName)
		if strings.HasSuffix(fileName, ".zip") {
			fmt.Printf("OTA_URL_RESULT: %s\n", directURL)
		}
		return nil
	}

	return fmt.Errorf("failed: %s", resp.Status)
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
	
	var latestZip string
	var zips []string
	for _, z := range zipFiles {
		if !strings.HasSuffix(strings.ToLower(z), "-ota.zip") {
			zips = append(zips, z)
		}
	}

	if len(zips) > 0 {
		sort.Slice(zips, func(i, j int) bool {
			fi, _ := os.Stat(zips[i])
			fj, _ := os.Stat(zips[j])
			return fi.ModTime().After(fj.ModTime())
		})
		latestZip = zips[0]
	}

	if latestZip == "" {
		fmt.Println("No ZIP found.")
		os.Exit(1)
	}

	tagName := time.Now().Format("20060102")
	zipExists, _ := fileAlreadyExists(tagName, filepath.Base(latestZip))

	if zipExists {
		fmt.Printf("⏭ ROM %s already exists. Skipping ZIP and all additional images (boot, vendor, etc).\n", filepath.Base(latestZip))
		url := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/packages/generic/builds/%s/%s", 
			projectID, tagName, filepath.Base(latestZip))
		fmt.Printf("OTA_URL_RESULT: %s\n", url)
		os.Exit(0)
	}

	uploadList := []string{latestZip}
	others := []string{"boot.img", "dtbo.img", "vendor_boot.img", "vendor_dlkm.img"}
	for _, img := range others {
		path := filepath.Join(baseDir, img)
		if _, err := os.Stat(path); err == nil {
			uploadList = append(uploadList, path)
		}
	}

	for _, f := range uploadList {
		gitlabUpload(f)
	}
}
