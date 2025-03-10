package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"ipm/pkg/log"
	"ipm/pkg/types"

	"github.com/Masterminds/semver/v3"
)

type Registry interface {
	FetchPackageTarball(name, version string) (io.ReadCloser, types.Package, error)
	ResolveVersion(name, versionRange string) (string, error)
}

type NPMRegistry struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func NewNPMRegistry(baseURL, token string) *NPMRegistry {
	return &NPMRegistry{
		BaseURL: baseURL,
		Token:   token,
		Client:  &http.Client{},
	}
}

func (r *NPMRegistry) FetchPackageTarball(name, version string) (io.ReadCloser, types.Package, error) {
	metadataURL := fmt.Sprintf("%s/%s/%s", r.BaseURL, name, version)
	log.Debug("Sending request to registry", map[string]interface{}{
		"url": metadataURL,
	})

	req, err := http.NewRequest("GET", metadataURL, nil)
	if err != nil {
		return nil, types.Package{}, fmt.Errorf("failed to create request: %v", err)
	}
	if r.Token != "" {
		req.Header.Set("Authorization", "Bearer "+r.Token)
	}

	resp, err := r.Client.Do(req)
	if err != nil {
		log.Error("Failed to fetch metadata", err, map[string]interface{}{
			"package": name,
			"version": version,
		})
		return nil, types.Package{}, fmt.Errorf("failed to fetch metadata for %s@%s: %v", name, version, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error("Metadata request failed", nil, map[string]interface{}{
			"status": resp.Status,
			"url":    metadataURL,
		})
		return nil, types.Package{}, fmt.Errorf("metadata request failed with status: %s", resp.Status)
	}

	var pkgData struct {
		Name         string            `json:"name"`
		Version      string            `json:"version"`
		Dist         struct {
			Tarball string `json:"tarball"`
		} `json:"dist"`
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pkgData); err != nil {
		log.Error("Failed to parse metadata", err, map[string]interface{}{
			"package": name,
			"version": version,
		})
		return nil, types.Package{}, fmt.Errorf("failed to parse metadata: %v", err)
	}

	log.Debug("Package metadata fetched", map[string]interface{}{
		"package": pkgData.Name,
		"version": pkgData.Version,
	})

	tarballReq, err := http.NewRequest("GET", pkgData.Dist.Tarball, nil)
	if err != nil {
		return nil, types.Package{}, fmt.Errorf("failed to create tarball request: %v", err)
	}
	if r.Token != "" {
		tarballReq.Header.Set("Authorization", "Bearer "+r.Token)
	}

	tarballResp, err := r.Client.Do(tarballReq)
	if err != nil {
		log.Error("Failed to fetch tarball", err, map[string]interface{}{
			"url": pkgData.Dist.Tarball,
		})
		return nil, types.Package{}, fmt.Errorf("failed to fetch tarball for %s@%s: %v", name, version, err)
	}
	if tarballResp.StatusCode != http.StatusOK {
		tarballResp.Body.Close()
		log.Error("Tarball request failed", nil, map[string]interface{}{
			"status": tarballResp.Status,
			"url":    pkgData.Dist.Tarball,
		})
		return nil, types.Package{}, fmt.Errorf("tarball request failed with status: %s", tarballResp.Status)
	}

	pkg := types.Package{
		Name:    pkgData.Name,
		Version: pkgData.Version,
		Deps:    pkgData.Dependencies,
	}
	return tarballResp.Body, pkg, nil
}

func (r *NPMRegistry) ResolveVersion(name, versionRange string) (string, error) {
	metadataURL := fmt.Sprintf("%s/%s", r.BaseURL, name)
	log.Debug("Sending request to registry for version resolution", map[string]interface{}{
		"url": metadataURL,
	})

	req, err := http.NewRequest("GET", metadataURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}
	if r.Token != "" {
		req.Header.Set("Authorization", "Bearer "+r.Token)
	}

	resp, err := r.Client.Do(req)
	if err != nil {
		log.Error("Failed to fetch metadata for version resolution", err, map[string]interface{}{
			"package": name,
		})
		return "", fmt.Errorf("failed to fetch metadata for %s: %v", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error("Metadata request failed", nil, map[string]interface{}{
			"status": resp.Status,
			"url":    metadataURL,
		})
		return "", fmt.Errorf("metadata request failed with status: %s", resp.Status)
	}

	var pkgData struct {
		Versions  map[string]interface{} `json:"versions"`
		DistTags  map[string]string      `json:"dist-tags"` // Hinzugefügt für dist-tags
	}
	if err := json.NewDecoder(resp.Body).Decode(&pkgData); err != nil {
		log.Error("Failed to parse metadata", err, map[string]interface{}{
			"package": name,
		})
		return "", fmt.Errorf("failed to parse metadata: %v", err)
	}

	// Behandle dist-tags wie "latest"
	if versionRange == "latest" {
		if latest, ok := pkgData.DistTags["latest"]; ok {
			log.Debug("Resolved dist-tag 'latest'", map[string]interface{}{
				"package": name,
				"version": latest,
			})
			return latest, nil
		}
		log.Error("No 'latest' dist-tag found", nil, map[string]interface{}{
			"package": name,
		})
		return "", fmt.Errorf("no 'latest' dist-tag found for %s", name)
	}

	constraint, err := semver.NewConstraint(versionRange)
	if err != nil {
		log.Error("Invalid version range", err, map[string]interface{}{
			"range": versionRange,
		})
		return "", fmt.Errorf("invalid version range %s: %v", versionRange, err)
	}

	var latest *semver.Version
	for verStr := range pkgData.Versions {
		ver, err := semver.NewVersion(verStr)
		if err != nil {
			continue
		}
		if constraint.Check(ver) {
			if latest == nil || ver.GreaterThan(latest) {
				latest = ver
			}
		}
	}

	if latest == nil {
		log.Error("No version found matching range", nil, map[string]interface{}{
			"package": name,
			"range":   versionRange,
		})
		return "", fmt.Errorf("no version found for %s matching %s", name, versionRange)
	}

	log.Debug("Version resolved", map[string]interface{}{
		"package": name,
		"range":   versionRange,
		"version": latest.Original(),
	})
	return latest.Original(), nil
}