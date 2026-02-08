package dictionary

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultDictFileName = "jmdict-eng-common.json"
	repoOwner           = "scriptin"
	repoName            = "jmdict-simplified"
)

// EnsureDictionary checks if the dictionary exists at path.
// If not, it discovers the latest release from GitHub, downloads it, and decompresses it.
func EnsureDictionary(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err == nil {
		// File exists
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	fmt.Printf("Dictionary not found at %s. Attempting auto-download...\n", path)

	downloadURL, err := getLatestReleaseAssetURL(ctx)
	if err != nil {
		return fmt.Errorf("failed to find latest dictionary release: %w", err)
	}

	fmt.Printf("Downloading from %s...\n", downloadURL)
	return downloadAndExtract(ctx, downloadURL, path)
}

func getLatestReleaseAssetURL(ctx context.Context) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}
	// Add User-Agent as required by GitHub API
	req.Header.Set("User-Agent", "readerer-cli")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api returned status: %s", resp.Status)
	}

	var release struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	// Find the English common dictionary
	// Pattern: jmdict-eng-common-*.json.tgz (or .json.gz if available, but .tgz is current)
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, "jmdict-eng-common") && (strings.HasSuffix(asset.Name, ".json.tgz") || strings.HasSuffix(asset.Name, ".json.gz")) {
			return asset.BrowserDownloadURL, nil
		}
	}

	return "", fmt.Errorf("no suitable dictionary asset found in latest release")
}

func downloadAndExtract(ctx context.Context, url, destPath string) error {
	// Create temp file for download
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	// Use a client with a generous timeout for the large file download
	client := &http.Client{
		Timeout: 30 * time.Minute,
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	// The file is likely gzipped or tar.gzipped.
	// We handle .tgz (tar.gz) which is the current format for jmdict-simplified.
	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Try treating it as a tar stream
	tarReader := tar.NewReader(gzReader)

	var found bool
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			// If it's not a tar file, we might get an error here or on NewReader.
			// But for now assuming .tgz
			return fmt.Errorf("error reading tar archive: %w", err)
		}

		if header.Typeflag == tar.TypeReg && strings.HasSuffix(header.Name, ".json") {
			// Found the JSON file
			outFile, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("failed to create output file: %w", err)
			}
			defer outFile.Close()

			if _, err := io.Copy(outFile, tarReader); err != nil {
				return fmt.Errorf("failed to write to file: %w", err)
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("no json file found in downloaded archive")
	}

	return nil
}
