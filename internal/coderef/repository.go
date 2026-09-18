package coderef

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// CanonicalRepository returns the portable identity persisted in manifests and
// code references. Authentication userinfo is deliberately discarded.
func CanonicalRepository(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() {
		return "", fmt.Errorf("repository must be an absolute URI")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("repository URI cannot contain a query or fragment")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.User = nil
	if parsed.Scheme == "file" {
		if parsed.Opaque != "" || parsed.Path == "" || !strings.HasPrefix(parsed.Path, "/") {
			return "", fmt.Errorf("file repository URI must have an absolute path")
		}
		parsed.Host = strings.ToLower(parsed.Host)
		parsed.Path = path.Clean(parsed.Path)
		parsed.RawPath = ""
		return parsed.String(), nil
	}
	if parsed.Opaque != "" {
		return "", fmt.Errorf("repository must be an absolute URI with a host")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("repository must be an absolute URI with a host")
	}
	host := strings.ToLower(parsed.Hostname())
	if port := parsed.Port(); port != "" {
		if !(parsed.Scheme == "https" && port == "443" || parsed.Scheme == "http" && port == "80" || parsed.Scheme == "ssh" && port == "22") {
			host = net.JoinHostPort(host, port)
		}
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	parsed.Host = host
	cleanPath := path.Clean("/" + strings.TrimPrefix(parsed.Path, "/"))
	if cleanPath != "/" {
		cleanPath = strings.TrimSuffix(cleanPath, "/")
	}
	parsed.Path = cleanPath
	parsed.RawPath = ""
	return parsed.String(), nil
}

// FileRepository constructs a canonical file repository URI for a local path.
func FileRepository(value string) (string, error) {
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	slash := filepath.ToSlash(filepath.Clean(abs))
	host := ""
	if runtime.GOOS == "windows" {
		if strings.HasPrefix(slash, "//") {
			parts := strings.SplitN(strings.TrimPrefix(slash, "//"), "/", 2)
			host = parts[0]
			slash = "/"
			if len(parts) == 2 {
				slash += parts[1]
			}
		} else if !strings.HasPrefix(slash, "/") {
			slash = "/" + slash
		}
	}
	return CanonicalRepository((&url.URL{Scheme: "file", Host: host, Path: slash}).String())
}

// RepositoryFilePath resolves a canonical file repository URI on this host.
func RepositoryFilePath(value string) (string, error) {
	canonical, err := CanonicalRepository(value)
	if err != nil {
		return "", err
	}
	parsed, _ := url.Parse(canonical)
	if parsed.Scheme != "file" {
		return "", fmt.Errorf("repository URI is not file://")
	}
	slash := parsed.Path
	if runtime.GOOS == "windows" {
		if parsed.Host != "" {
			slash = "//" + parsed.Host + slash
		} else if len(slash) >= 3 && slash[0] == '/' && slash[2] == ':' {
			slash = slash[1:]
		}
	} else if parsed.Host != "" {
		slash = "//" + parsed.Host + slash
	}
	local := filepath.Clean(filepath.FromSlash(slash))
	if !filepath.IsAbs(local) {
		return "", fmt.Errorf("file repository URI does not resolve to an absolute local path")
	}
	return local, nil
}
