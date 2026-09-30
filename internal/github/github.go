package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type Runner func(ctx context.Context, args ...string) ([]byte, error)

type PRRef struct {
	Host    string
	Project string
	Number  int
}

func (r PRRef) Path(suffix string) string {
	return fmt.Sprintf("repos/%s/pulls/%d%s", r.Project, r.Number, suffix)
}

var prURL = regexp.MustCompile(`^https?://([^/]+)/([^/]+/[^/]+)/pull/(\d+)`)

func IsPRURL(s string) bool { return prURL.MatchString(s) }

func ParsePRURL(raw string) (PRRef, error) {
	m := prURL.FindStringSubmatch(raw)
	if m == nil {
		return PRRef{}, fmt.Errorf("not a pull request URL: %q", raw)
	}
	n, err := strconv.Atoi(m[3])
	if err != nil || n <= 0 {
		return PRRef{}, fmt.Errorf("not a pull request URL: %q", raw)
	}
	return PRRef{Host: m[1], Project: m[2], Number: n}, nil
}

func Gh(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, errors.New("gh not found: install it from https://cli.github.com")
	}
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s (not logged in? run: gh auth login)",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

type PR struct {
	Title string `json:"title"`
	URL   string `json:"html_url"`
	Head  struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		SHA string `json:"sha"`
	} `json:"base"`
}

func FetchPR(ctx context.Context, run Runner, ref PRRef) (PR, error) {
	out, err := run(ctx, "api", "--hostname", ref.Host, ref.Path(""))
	if err != nil {
		return PR{}, err
	}
	var pr PR
	if err := json.Unmarshal(out, &pr); err != nil {
		return PR{}, fmt.Errorf("decode pull request: %w", err)
	}
	if pr.Head.SHA == "" || pr.Base.SHA == "" {
		return PR{}, errors.New("pull request has no head or base commit")
	}
	return pr, nil
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
	ReplyTo    int64
}

type Note struct {
	Author string
	Body   string
}

const threadsQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: 100) {
        nodes {
          id isResolved path line originalLine diffSide
          comments(first: 50) { totalCount nodes { databaseId body author { login } } }
        }
      }
      comments(first: 100) { nodes { id body author { login } } }
    }
  }
}`

type comment struct {
	ID         string `json:"id"`
	DatabaseID int64  `json:"databaseId"`
	Body       string `json:"body"`
	Author     struct {
		Login string `json:"login"`
	} `json:"author"`
}

type threadsReply struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					Nodes []struct {
						ID           string `json:"id"`
						IsResolved   bool   `json:"isResolved"`
						Path         string `json:"path"`
						Line         *int   `json:"line"`
						OriginalLine *int   `json:"originalLine"`
						DiffSide     string `json:"diffSide"`
						Comments     struct {
							TotalCount int       `json:"totalCount"`
							Nodes      []comment `json:"nodes"`
						} `json:"comments"`
					} `json:"nodes"`
				} `json:"reviewThreads"`
				Comments struct {
					Nodes []comment `json:"nodes"`
				} `json:"comments"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

var htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)

func FetchDiscussions(ctx context.Context, run Runner, ref PRRef) ([]Discussion, error) {
	owner, name, _ := strings.Cut(ref.Project, "/")
	out, err := run(ctx, "api", "--hostname", ref.Host, "graphql",
		"-f", "query="+threadsQuery, "-F", "owner="+owner, "-F", "name="+name,
		"-F", fmt.Sprintf("number=%d", ref.Number))
	if err != nil {
		return nil, err
	}
	var reply threadsReply
	if err := json.Unmarshal(out, &reply); err != nil {
		return nil, fmt.Errorf("decode review threads: %w", err)
	}
	pr := reply.Data.Repository.PullRequest
	var result []Discussion
	for _, t := range pr.ReviewThreads.Nodes {
		if len(t.Comments.Nodes) == 0 {
			continue
		}
		first := t.Comments.Nodes[0]
		body := strings.TrimSpace(htmlComment.ReplaceAllString(first.Body, ""))
		if body == "" {
			continue
		}
		d := Discussion{
			ID: t.ID, Author: first.Author.Login, Body: body, File: t.Path,
			Replies: t.Comments.TotalCount - 1, Resolved: t.IsResolved,
			ReplyTo: first.DatabaseID, Resolvable: true,
		}
		for _, c := range t.Comments.Nodes {
			d.Notes = append(d.Notes, Note{Author: c.Author.Login, Body: c.Body})
		}
		switch {
		case t.Line != nil && t.DiffSide != "LEFT":
			d.Line = *t.Line
		case t.OriginalLine != nil:
			d.Line, d.OldLine = *t.OriginalLine, true
		}
		result = append(result, d)
	}
	for _, c := range pr.Comments.Nodes {
		if body := strings.TrimSpace(htmlComment.ReplaceAllString(c.Body, "")); body != "" {
			result = append(result, Discussion{ID: c.ID, Author: c.Author.Login, Body: body})
		}
	}
	return result, nil
}

func CurrentUser(ctx context.Context, run Runner, host string) (string, error) {
	out, err := run(ctx, "api", "--hostname", host, "user")
	if err != nil {
		return "", err
	}
	var u struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(out, &u); err != nil {
		return "", fmt.Errorf("decode user: %w", err)
	}
	return u.Login, nil
}

type ReviewComment struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Side      string `json:"side"`
	StartLine int    `json:"start_line,omitempty"`
	StartSide string `json:"start_side,omitempty"`
	Body      string `json:"body"`
}

type Review struct {
	CommitID string          `json:"commit_id"`
	Body     string          `json:"body,omitempty"`
	Event    string          `json:"event"`
	Comments []ReviewComment `json:"comments,omitempty"`
}
