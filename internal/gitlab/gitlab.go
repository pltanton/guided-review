package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type MRRef struct {
	Host    string
	Project string
	IID     int
}

type DiffRefs struct {
	BaseSHA  string `json:"base_sha"`
	StartSHA string `json:"start_sha"`
	HeadSHA  string `json:"head_sha"`
}

type MR struct {
	Title        string   `json:"title"`
	WebURL       string   `json:"web_url"`
	SourceBranch string   `json:"source_branch"`
	DiffRefs     DiffRefs `json:"diff_refs"`
}

type Runner func(ctx context.Context, args ...string) ([]byte, error)

const mrPathSep = "/-/merge_requests/"

var htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)

func IsMRURL(s string) bool {
	return strings.Contains(s, mrPathSep)
}

func ParseMRURL(raw string) (MRRef, error) {
	bad := fmt.Errorf("not a merge request URL: %q", raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return MRRef{}, bad
	}
	project, rest, ok := strings.Cut(strings.Trim(u.Path, "/"), strings.TrimPrefix(mrPathSep, "/"))
	project = strings.TrimSuffix(project, "/")
	if !ok || project == "" {
		return MRRef{}, bad
	}
	iidText, _, _ := strings.Cut(rest, "/")
	iid, err := strconv.Atoi(iidText)
	if err != nil || iid <= 0 {
		return MRRef{}, bad
	}
	return MRRef{Host: u.Host, Project: project, IID: iid}, nil
}

func Glab(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "glab", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, errors.New("glab not found: install it from https://gitlab.com/gitlab-org/cli")
	}
	if err != nil {
		return nil, fmt.Errorf("glab %s: %w: %s (not logged in? run: glab auth login)",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func FetchMR(ctx context.Context, run Runner, ref MRRef) (MR, error) {
	path := fmt.Sprintf("projects/%s/merge_requests/%d", url.PathEscape(ref.Project), ref.IID)
	out, err := run(ctx, "api", "--hostname", ref.Host, path)
	if err != nil {
		return MR{}, err
	}
	var mr MR
	if err := json.Unmarshal(out, &mr); err != nil {
		return MR{}, fmt.Errorf("decode merge request: %w", err)
	}
	if mr.DiffRefs.HeadSHA == "" {
		return MR{}, errors.New("merge request has no diff_refs yet")
	}
	return mr, nil
}

type Discussion struct {
	ID         string
	Author     string
	Body       string
	Replies    int
	File       string
	Line       int
	OldLine    bool
	Resolved   bool
	Resolvable bool
	Notes      []Note
}

type Note struct {
	Author string
	Body   string
}

type apiDiscussion struct {
	ID    string    `json:"id"`
	Notes []apiNote `json:"notes"`
}

type apiNote struct {
	Body   string `json:"body"`
	System bool   `json:"system"`
	Author struct {
		Username string `json:"username"`
	} `json:"author"`
	Resolved   bool         `json:"resolved"`
	Resolvable bool         `json:"resolvable"`
	Position   *apiPosition `json:"position"`
}

type apiPosition struct {
	NewPath string `json:"new_path"`
	NewLine *int   `json:"new_line"`
	OldPath string `json:"old_path"`
	OldLine *int   `json:"old_line"`
}

func FetchDiscussions(ctx context.Context, run Runner, ref MRRef) ([]Discussion, error) {
	path := fmt.Sprintf(
		"projects/%s/merge_requests/%d/discussions?per_page=100",
		url.PathEscape(ref.Project),
		ref.IID,
	)
	out, err := run(ctx, "api", "--hostname", ref.Host, "--paginate", path)
	if err != nil {
		return nil, err
	}
	var result []Discussion
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var page []apiDiscussion
		if err := dec.Decode(&page); err != nil {
			return nil, fmt.Errorf("decode discussions: %w", err)
		}
		for _, d := range page {
			if len(d.Notes) == 0 || d.Notes[0].System {
				continue
			}
			first := d.Notes[0]
			body := strings.TrimSpace(htmlComment.ReplaceAllString(first.Body, ""))
			if body == "" {
				continue
			}
			disc := Discussion{
				ID:         d.ID,
				Author:     first.Author.Username,
				Body:       body,
				Replies:    len(d.Notes) - 1,
				Resolved:   first.Resolved,
				Resolvable: first.Resolvable,
			}
			for _, n := range d.Notes {
				if !n.System {
					disc.Notes = append(disc.Notes, Note{Author: n.Author.Username, Body: n.Body})
				}
			}
			if p := first.Position; p != nil {
				switch {
				case p.NewLine != nil:
					disc.File, disc.Line = p.NewPath, *p.NewLine
				case p.OldLine != nil:
					disc.File, disc.Line, disc.OldLine = p.OldPath, *p.OldLine, true
				}
			}
			result = append(result, disc)
		}
	}
	return result, nil
}

type Position struct {
	PositionType string `json:"position_type"`
	BaseSHA      string `json:"base_sha"`
	StartSHA     string `json:"start_sha"`
	HeadSHA      string `json:"head_sha"`
	OldPath      string `json:"old_path"`
	NewPath      string `json:"new_path"`
	NewLine      int    `json:"new_line,omitempty"`
	OldLine      int    `json:"old_line,omitempty"`
}

type DraftNote struct {
	Note     string    `json:"note"`
	Position *Position `json:"position,omitempty"`
}

func CurrentUser(ctx context.Context, run Runner, host string) (string, error) {
	out, err := run(ctx, "api", "--hostname", host, "user")
	if err != nil {
		return "", err
	}
	var u struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(out, &u); err != nil {
		return "", fmt.Errorf("decode user: %w", err)
	}
	return u.Username, nil
}

func (r MRRef) Path(suffix string) string {
	return fmt.Sprintf("projects/%s/merge_requests/%d%s", url.PathEscape(r.Project), r.IID, suffix)
}
