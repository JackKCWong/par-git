package main

import (
	"testing"
)

func TestParseOrgURL(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantHost  string
		wantOrg   string
		wantErr   bool
	}{
		{
			name:     "valid github.com https",
			input:    "https://github.com/hsbc",
			wantHost: "github.com",
			wantOrg:  "hsbc",
		},
		{
			name:     "valid github.com http",
			input:    "http://github.com/hsbc",
			wantHost: "github.com",
			wantOrg:  "hsbc",
		},
		{
			name:     "valid GHES custom host",
			input:    "https://github.mycompany.com/team",
			wantHost: "github.mycompany.com",
			wantOrg:  "team",
		},
		{
			name:     "trailing slash",
			input:    "https://github.com/hsbc/",
			wantHost: "github.com",
			wantOrg:  "hsbc",
		},
		{
			name:     "hyphenated name",
			input:    "https://github.com/my-org",
			wantHost: "github.com",
			wantOrg:  "my-org",
		},
		{
			name:     "single char name",
			input:    "https://github.com/a",
			wantHost: "github.com",
			wantOrg:  "a",
		},
		{
			name:     "all-digits name",
			input:    "https://github.com/12345",
			wantHost: "github.com",
			wantOrg:  "12345",
		},
		{
			name:     "max length (39 chars)",
			input:    "https://github.com/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			wantHost: "github.com",
			wantOrg:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "missing scheme",
			input:   "github.com/hsbc",
			wantErr: true,
		},
		{
			name:    "unsupported scheme",
			input:   "ftp://github.com/hsbc",
			wantErr: true,
		},
		{
			name:    "missing host",
			input:   "https:///hsbc",
			wantErr: true,
		},
		{
			name:    "missing path",
			input:   "https://github.com",
			wantErr: true,
		},
		{
			name:    "repo-style path",
			input:   "https://github.com/hsbc/repo",
			wantErr: true,
		},
		{
			name:    "invalid characters (dot)",
			input:   "https://github.com/hs.bc",
			wantErr: true,
		},
		{
			name:    "path traversal",
			input:   "https://github.com/..",
			wantErr: true,
		},
		{
			name:    "leading hyphen",
			input:   "https://github.com/-hsbc",
			wantErr: true,
		},
		{
			name:    "trailing hyphen",
			input:   "https://github.com/hsbc-",
			wantErr: true,
		},
		{
			name:    "too long (40 chars)",
			input:   "https://github.com/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			wantErr: true,
		},
		{
			name:    "userinfo present",
			input:   "https://user:token@github.com/hsbc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, org, err := parseOrgURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseOrgURL(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if host != tt.wantHost {
				t.Errorf("parseOrgURL(%q) host = %q, want %q", tt.input, host, tt.wantHost)
			}
			if org != tt.wantOrg {
				t.Errorf("parseOrgURL(%q) org = %q, want %q", tt.input, org, tt.wantOrg)
			}
		})
	}
}

func TestHasNextPage(t *testing.T) {
	tests := []struct {
		name string
		link string
		want bool
	}{
		{
			name: "empty",
			link: "",
			want: false,
		},
		{
			name: "single next",
			link: `<https://api.github.com/orgs/foo/repos?page=2>; rel="next"`,
			want: true,
		},
		{
			name: "next and last",
			link: `<https://api.github.com/orgs/foo/repos?page=2>; rel="next", <https://api.github.com/orgs/foo/repos?page=10>; rel="last"`,
			want: true,
		},
		{
			name: "last only",
			link: `<https://api.github.com/orgs/foo/repos?page=10>; rel="last"`,
			want: false,
		},
		{
			name: "prev only",
			link: `<https://api.github.com/orgs/foo/repos?page=1>; rel="prev"`,
			want: false,
		},
		{
			name: "prev, next, last in order",
			link: `<https://...?page=1>; rel="prev", <https://...?page=3>; rel="next", <https://...?page=10>; rel="last"`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasNextPage(tt.link); got != tt.want {
				t.Errorf("hasNextPage(%q) = %v, want %v", tt.link, got, tt.want)
			}
		})
	}
}

func TestIsNonGitHubHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want bool
	}{
		{name: "github.com", host: "github.com", want: false},
		{name: "GHES custom domain", host: "github.mycompany.com", want: false},
		{name: "GHES subdomain", host: "ghe.example.org", want: false},
		{name: "gitlab.com", host: "gitlab.com", want: true},
		{name: "bitbucket.org", host: "bitbucket.org", want: true},
		{name: "codeberg.org", host: "codeberg.org", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNonGitHubHost(tt.host); got != tt.want {
				t.Errorf("isNonGitHubHost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}
