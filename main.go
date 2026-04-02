package main

import (
        "fmt"
        "io"
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

func gitlabUpload(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	fileName := filepath.Base(filePath)
	tagName := time.Now().Format("20060102")

	uploadURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/packages/generic/builds/%s/%s", 
		projectID, tagName, fileName)

	displayURL := fmt.Sprintf("https://gitlab.com/ruri/ayaka-releases/-/package_files/latest/download?filename=%s", fileName)

	directURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/ruri%%2Fayaka-releases/packages/generic/builds/%s/%s", tagName, fileName)

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
		fmt.Printf("🔗 URL: %s\n", directURL)
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("Error: %s - %s", resp.Status, string(body))
}

func main() {
        if len(os.Args) != 2 {
                fmt.Println("Usage: ./lab <device>")
                os.Exit(1)
        }

        device := os.Args[1]
        baseDir := fmt.Sprintf("out/target/product/%s", device)

        var uploadFiles []string

        zipPattern := filepath.Join(baseDir, "*.zip")
        zipFiles, _ := filepath.Glob(zipPattern)

        var zips []string
        for _, z := range zipFiles {
                if strings.HasSuffix(strings.ToLower(z), "-ota.zip") {
                        continue
                }
                zips = append(zips, z)
        }

        if len(zips) > 0 {
                sort.Slice(zips, func(i, j int) bool {
                        fi, _ := os.Stat(zips[i])
                        fj, _ := os.Stat(zips[j])
                        return fi.ModTime().After(fj.ModTime())
                })
                uploadFiles = append(uploadFiles, zips[0])
        }

        otherPatterns := []string{
                filepath.Join(baseDir, "dtbo.img"),
                filepath.Join(baseDir, "vendor_boot.img"),
                filepath.Join(baseDir, "boot.img"),
                filepath.Join(baseDir, "vendor_dlkm.img"),
        }

        for _, pattern := range otherPatterns {
                matches, _ := filepath.Glob(pattern)
                uploadFiles = append(uploadFiles, matches...)
        }

        if len(uploadFiles) == 0 {
                fmt.Println("No files found for the device.:", device)
                os.Exit(0)
        }

        fmt.Println("Starting upload to GitLab...")
        for _, file := range uploadFiles {
                fmt.Printf("Sending: %s...\n", filepath.Base(file))
                if err := gitlabUpload(file); err != nil {
                        fmt.Printf("Error in file %s: %v\n", file, err)
                }
        }
        fmt.Println("Process completed!")
}
