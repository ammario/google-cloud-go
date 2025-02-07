// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/postprocessor/execv/gocmd"
	"gopkg.in/yaml.v3"
)

const betaIndicator = "It is not stable"

// ManifestEntry is used for JSON marshaling in manifest.
type ManifestEntry struct {
	DistributionName  string      `json:"distribution_name" yaml:"distribution-name"`
	Description       string      `json:"description" yaml:"description"`
	Language          string      `json:"language" yaml:"language"`
	ClientLibraryType string      `json:"client_library_type" yaml:"client-library-type"`
	DocsURL           string      `json:"docs_url" yaml:"docs-url"`
	ReleaseLevel      string      `json:"release_level" yaml:"release-level"`
	LibraryType       libraryType `json:"library_type" yaml:"library-type"`
}

type libraryType string

const (
	gapicAutoLibraryType   libraryType = "GAPIC_AUTO"
	gapicManualLibraryType libraryType = "GAPIC_MANUAL"
	coreLibraryType        libraryType = "CORE"
	agentLibraryType       libraryType = "AGENT"
	otherLibraryType       libraryType = "OTHER"
)

// Manifest writes a manifest file with info about all of the confs.
func (p *postProcessor) Manifest() (map[string]ManifestEntry, error) {
	log.Println("updating gapic manifest")
	entries := map[string]ManifestEntry{} // Key is the package name.
	f, err := os.Create(filepath.Join(p.googleCloudDir, "internal", ".repo-metadata-full.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	for _, manual := range p.config.ManualClientInfo {
		entries[manual.DistributionName] = *manual
	}
	for inputDir, conf := range p.config.GoogleapisToImportPath {
		if conf.ServiceConfig == "" {
			continue
		}
		yamlPath := filepath.Join(p.googleapisDir, inputDir, conf.ServiceConfig)
		yamlFile, err := os.Open(yamlPath)
		if err != nil {
			return nil, err
		}
		yamlConfig := struct {
			Title string `yaml:"title"` // We only need the title field.
		}{}
		if err := yaml.NewDecoder(yamlFile).Decode(&yamlConfig); err != nil {
			return nil, fmt.Errorf("decode: %v", err)
		}
		docURL, err := docURL(p.googleCloudDir, conf.ImportPath, conf.RelPath)
		if err != nil {
			return nil, fmt.Errorf("unable to build docs URL: %v", err)
		}
		releaseLevel, err := releaseLevel(p.googleCloudDir, conf.ImportPath, conf.RelPath)
		if err != nil {
			return nil, fmt.Errorf("unable to calculate release level for %v: %v", inputDir, err)
		}

		entry := ManifestEntry{
			DistributionName:  conf.ImportPath,
			Description:       yamlConfig.Title,
			Language:          "Go",
			ClientLibraryType: "generated",
			DocsURL:           docURL,
			ReleaseLevel:      releaseLevel,
			LibraryType:       gapicAutoLibraryType,
		}
		entries[conf.ImportPath] = entry
	}
	// Remove base module entry
	delete(entries, "")
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return entries, enc.Encode(entries)
}

func docURL(cloudDir, importPath, relPath string) (string, error) {
	dir := filepath.Join(cloudDir, relPath)
	mod, err := gocmd.CurrentMod(dir)
	if err != nil {
		return "", err
	}
	pkgPath := strings.TrimPrefix(strings.TrimPrefix(importPath, mod), "/")
	return "https://cloud.google.com/go/docs/reference/" + mod + "/latest/" + pkgPath, nil
}

func releaseLevel(cloudDir, importPath, relPath string) (string, error) {
	i := strings.LastIndex(importPath, "/")
	lastElm := importPath[i+1:]
	if strings.Contains(lastElm, "alpha") {
		return "alpha", nil
	} else if strings.Contains(lastElm, "beta") {
		return "beta", nil
	}

	// Determine by scanning doc.go for our beta disclaimer
	docFile := filepath.Join(cloudDir, relPath, "doc.go")
	f, err := os.Open(docFile)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lineCnt int
	for scanner.Scan() && lineCnt < 50 {
		line := scanner.Text()
		if strings.Contains(line, betaIndicator) {
			return "beta", nil
		}
	}
	return "ga", nil
}
