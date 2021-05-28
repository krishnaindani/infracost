package schema

import (
	"crypto/md5" // nolint:gosec
	"encoding/base32"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
)

type ProjectMetadata struct {
	Path               string `json:"path"`
	Type               string `json:"type"`
	VCSRepoURL         string `json:"vcsRepoUrl,omitempty"`
	VCSSubPath         string `json:"vcsSubPath,omitempty"`
	VCSPullRequestURL  string `json:"vcsPullRequestUrl,omitempty"`
	TerraformWorkspace string `json:"terraformWorkspace,omitempty"`
}

// Project contains the existing, planned state of
// resources and the diff between them.
type Project struct {
	Name          string
	Metadata      *ProjectMetadata
	PastResources []*Resource
	Resources     []*Resource
	Diff          []*Resource
	HasDiff       bool
}

func NewProject(name string, metadata *ProjectMetadata) *Project {
	return &Project{
		Name:     name,
		Metadata: metadata,
		HasDiff:  true,
	}
}

// AllResources returns a pointer list of all resources of the state.
func (p *Project) AllResources() []*Resource {
	var resources []*Resource
	resources = append(resources, p.PastResources...)
	resources = append(resources, p.Resources...)
	return resources
}

// CalculateDiff calculates the diff of past and current resources
func (p *Project) CalculateDiff() {
	if p.HasDiff {
		p.Diff = calculateDiff(p.PastResources, p.Resources)
	}
}

// AllProjectResources returns the resources for all projects
func AllProjectResources(projects []*Project) []*Resource {
	resources := make([]*Resource, 0)

	for _, p := range projects {
		resources = append(resources, p.Resources...)
	}

	return resources
}

func GenerateProjectName(metadata *ProjectMetadata) string {
	var n string

	// If the VCS repo is set, create the name from that
	if metadata.VCSRepoURL != "" {
		n = tidyRepoURL(metadata.VCSRepoURL)

		if metadata.VCSSubPath != "" {
			n += "/" + metadata.VCSSubPath
		}
		// If not then use a hash of the absolute filepath to the project
	} else {
		absPath, err := filepath.Abs(metadata.Path)
		if err != nil {
			log.Debugf("Could not get absolute path for %s", metadata.Path)
			absPath = metadata.Path
		}

		n = fmt.Sprintf("project_%s", shortHash(absPath, 8))
	}

	if metadata.TerraformWorkspace != "" && metadata.TerraformWorkspace != "default" {
		n += fmt.Sprintf(" (%s)", metadata.TerraformWorkspace)
	}

	return n
}

// Strips the *@ and .git prefix and suffix
func tidyRepoURL(url string) string {
	r := regexp.MustCompile(`(?:\w+@|https:\/\/)([^:\/]+)[:\/](.+)(?:\.git)`)
	m := r.FindStringSubmatch(url)
	if len(m) > 2 {
		s := ""

		if strings.ToLower(m[1]) == "github.com" {
			s += "GitHub: "
		} else if strings.ToLower(m[1]) == "gitlab.com" {
			s += "GitLab: "
		} else {
			s += m[1] + ": "
		}

		s += m[2]
		return s
	}

	return url
}

// Returns a lowercase truncated hash of length l
func shortHash(s string, l int) string {
	sum := md5.Sum([]byte(s)) //nolint:gosec
	var b []byte = sum[:]
	h := base32.StdEncoding.EncodeToString(b)

	return strings.ToLower(h)[:l]
}
