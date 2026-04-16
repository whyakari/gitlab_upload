package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
		log.Fatal("GITLAB_TOKEN or PROJECTID_GITLAB not defined")
	}
}

// Verifica se o arquivo já existe dentro de um pacote específico
func fileAlreadyExists(packageName, fileName string) (bool, error) {
	// Procuramos pelo package que tem o nome do ZIP
	url := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/packages?package_name=builds&package_version=%s",
		projectID, packageName)

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
	if respFile != nil {
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

	// Localizar o ZIP mais recente
	zipPattern := filepath.Join(baseDir, "*.zip")
	zipFiles, _ := filepath.Glob(zipPattern)

	var parsed []zipInfo
	for _, z := range zipFiles {
		base := filepath.Base(z)
		if strings.HasSuffix(strings.ToLower(base), "-ota.zip") || strings.Contains(base, "target_files") {
			continue
		}

		parts := strings.Split(base, "-")
		if len(parts) < 3 {
			continue
		}

		// Tenta extrair timestamp do nome do arquivo
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
	
	// ESTRATÉGIA: A versão do pacote será o nome do ZIP sem .zip
	// Exemplo: AyakaUI-device-v1.0-OFFICIAL-20240505
	packageVersion := strings.TrimSuffix(zipName, ".zip")

	// Definir arquivos adicionais (Boot e Vendor Boot)
	bootImg := filepath.Join(baseDir, "boot.img")
	vendorBootImg := filepath.Join(baseDir, "vendor_boot.img")

	filesToUpload := []string{latestZip}
	
	// Adiciona imagens se existirem
	if _, err := os.Stat(bootImg); err == nil {
		filesToUpload = append(filesToUpload, bootImg)
	}
	if _, err := os.Stat(vendorBootImg); err == nil {
		filesToUpload = append(filesToUpload, vendorBootImg)
	}

	client := &http.Client{}
	for _, f := range filesToUpload {
		currName := filepath.Base(f)
		
		// Verifica se este arquivo específico já está no pacote desta build
		exists, _ := fileAlreadyExists(packageVersion, currName)
		if exists {
			fmt.Printf("⏭ %s already exists in package %s. Skipping.\n", currName, packageVersion)
			if strings.HasSuffix(currName, ".zip") {
				fmt.Printf("OTA_URL_RESULT: https://gitlab.com/api/v4/projects/%s/packages/generic/builds/%s/%s\n", 
					projectID, packageVersion, currName)
			}
			continue
		}

		fmt.Printf("📤 Uploading: %s to package %s...\n", currName, packageVersion)
		
		file, err := os.Open(f)
		if err != nil {
			fmt.Printf("❌ Error opening %s: %v\n", currName, err)
			continue
		}

		// URL agora usa packageVersion (único por build) em vez da data genérica
		uploadURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/packages/generic/builds/%s/%s", 
			projectID, packageVersion, currName)

		req, _ := http.NewRequest("PUT", uploadURL, file)
		req.Header.Set("PRIVATE-TOKEN", gitlabToken)

		resp, err := client.Do(req)
		if err == nil && (resp.StatusCode == 201 || resp.StatusCode == 200) {
			fmt.Printf("✅ %s uploaded successfully.\n", currName)
			if strings.HasSuffix(currName, ".zip") {
				fmt.Printf("OTA_URL_RESULT: %s\n", uploadURL)
			}
			resp.Body.Close()
		} else {
			status := "unknown"
			if resp != nil { status = resp.Status }
			fmt.Printf("❌ Failed to upload %s (Status: %s)\n", currName, status)
		}
		file.Close()
	}
}
