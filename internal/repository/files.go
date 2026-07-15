package repository

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/yjydist/traceframe/internal/domain"
)

var (
	manifestNames    = []string{"Cargo.toml", "Gemfile", "go.mod", "package.json", "pom.xml", "pyproject.toml", "requirements.txt"}
	binaryExtensions = map[string]struct{}{
		".7z": {}, ".a": {}, ".avi": {}, ".bin": {}, ".bmp": {}, ".class": {}, ".db": {}, ".dll": {}, ".dylib": {}, ".exe": {},
		".gif": {}, ".gz": {}, ".ico": {}, ".jar": {}, ".jpeg": {}, ".jpg": {}, ".mov": {}, ".mp3": {}, ".mp4": {}, ".o": {},
		".pdf": {}, ".png": {}, ".so": {}, ".sqlite": {}, ".tar": {}, ".ttf": {}, ".wav": {}, ".webm": {}, ".woff": {}, ".woff2": {}, ".zip": {},
	}
	secretExtensions = map[string]struct{}{".der": {}, ".jks": {}, ".key": {}, ".kdbx": {}, ".p12": {}, ".pem": {}, ".pfx": {}}
	credentialValue  = regexp.MustCompile(`(?i)(?m)(api[_-]?key|access[_-]?token|auth[_-]?token|client[_-]?secret|password|private[_-]?key|secret)\s*[:=]\s*([^\s,;]+)`)
	bearerValue      = regexp.MustCompile(`(?i)bearer\s+[a-z0-9._~+/=-]{8,}`)
	urlCredential    = regexp.MustCompile(`(?i)(https?://[^\s/:]+:)[^@\s]+@`)
	knownTokenValue  = regexp.MustCompile(`(?i)\b(gh[pousr]_[a-z0-9]{20,}|github_pat_[a-z0-9_]{20,}|sk-[a-z0-9_-]{16,}|AKIA[0-9A-Z]{16}|eyJ[a-z0-9_-]{8,}\.[a-z0-9_-]{8,}\.[a-z0-9_-]{8,})\b`)
)

type ignoreRule struct {
	pattern string
	negate  bool
	dirOnly bool
}

func (s *Service) listFiles(grant Grant, requestedPath string, limit int, testsOnly bool) ([]Entry, bool, error) {
	start, err := s.resolvePath(grant, requestedPath)
	if err != nil {
		return nil, false, err
	}
	info, err := os.Stat(start)
	if err != nil || !info.IsDir() {
		return nil, false, fmt.Errorf("%w: list_files path must be a directory", domainInvalid())
	}
	rules := loadIgnoreRules(grant.CanonicalRoot)
	entries := make([]Entry, 0, limit)
	walked := 0
	truncated := false
	err = filepath.WalkDir(start, func(current string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if current == start {
			return nil
		}
		rel, relErr := filepath.Rel(grant.CanonicalRoot, current)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if item.Type()&os.ModeSymlink != 0 {
			if item.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldIgnore(rel, item.IsDir(), rules) {
			if item.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if item.IsDir() {
			return nil
		}
		walked++
		if walked > s.options.MaxWalkFiles {
			truncated = true
			return filepath.SkipAll
		}
		if testsOnly && !isTestFile(rel) {
			return nil
		}
		data, fileInfo, readErr := s.readAllowedFile(grant, rel)
		if readErr != nil {
			return nil
		}
		entries = append(entries, Entry{Path: rel, Kind: map[bool]string{true: "test", false: "file"}[testsOnly], Size: fileInfo.Size(), SHA256: fileChecksum(data), Summary: fmt.Sprintf("%s (%d bytes)", rel, fileInfo.Size())})
		if len(entries) >= limit {
			truncated = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("walk repository: %w", err)
	}
	slices.SortFunc(entries, func(a, b Entry) int { return strings.Compare(a.Path, b.Path) })
	return entries, truncated, nil
}

func (s *Service) searchText(grant Grant, requestedPath, query string, limit int) ([]Entry, bool, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 200 || strings.ContainsRune(query, '\x00') {
		return nil, false, fmt.Errorf("%w: search query must contain 1 to 200 characters", domainInvalid())
	}
	files, filesTruncated, err := s.listFiles(grant, requestedPath, s.options.MaxWalkFiles, false)
	if err != nil {
		return nil, false, err
	}
	results := make([]Entry, 0, min(limit, 20))
	resultBytes := 0
	for _, file := range files {
		data, info, readErr := s.readAllowedFile(grant, file.Path)
		if readErr != nil {
			continue
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 64*1024), int(s.options.MaxFileBytes))
		line := 0
		for scanner.Scan() {
			line++
			text := scanner.Text()
			if !strings.Contains(text, query) {
				continue
			}
			redacted := redactCredentials(text)
			resultBytes += len(redacted)
			if resultBytes > s.options.MaxResultBytes || len(results) >= limit {
				return results, true, nil
			}
			results = append(results, Entry{Path: file.Path, Kind: "match", Size: info.Size(), SHA256: fileChecksum(data), StartLine: line, EndLine: line, Content: redacted, Summary: redacted})
		}
	}
	return results, filesTruncated, nil
}

func (s *Service) readFile(grant Grant, requestedPath string, startLine, endLine int) ([]Entry, error) {
	data, info, err := s.readAllowedFile(grant, requestedPath)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	if startLine <= 0 {
		startLine = 1
	}
	if endLine <= 0 || endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine > endLine || startLine > len(lines) {
		return nil, fmt.Errorf("%w: requested line range is outside the file", domainInvalid())
	}
	content := redactCredentials(strings.Join(lines[startLine-1:endLine], "\n"))
	if len(content) > s.options.MaxResultBytes {
		return nil, fmt.Errorf("%w: requested excerpt exceeds the result byte limit", domainInvalid())
	}
	rel, _ := s.relativePath(grant, requestedPath)
	return []Entry{{Path: rel, Kind: "excerpt", Size: info.Size(), SHA256: fileChecksum(data), StartLine: startLine, EndLine: endLine, Content: content, Summary: content}}, nil
}

func (s *Service) inspectManifests(grant Grant, requestedPath string, limit int) ([]Entry, bool, error) {
	files, truncated, err := s.listFiles(grant, requestedPath, s.options.MaxWalkFiles, false)
	if err != nil {
		return nil, false, err
	}
	results := make([]Entry, 0)
	for _, file := range files {
		if !slices.Contains(manifestNames, path.Base(file.Path)) {
			continue
		}
		entries, readErr := s.readFile(grant, file.Path, 1, 300)
		if readErr != nil {
			continue
		}
		entries[0].Kind = "manifest"
		results = append(results, entries[0])
		if len(results) >= limit {
			return results, true, nil
		}
	}
	return results, truncated, nil
}

func (s *Service) inspectGitMetadata(grant Grant) ([]Entry, error) {
	gitDir := filepath.Join(grant.CanonicalRoot, ".git")
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: repository has no readable .git directory", domainInvalid())
	}
	paths := []string{".git/HEAD", ".git/config"}
	if head, readErr := os.ReadFile(filepath.Join(gitDir, "HEAD")); readErr == nil {
		ref := strings.TrimSpace(strings.TrimPrefix(string(head), "ref:"))
		if ref != "" && ref != strings.TrimSpace(string(head)) && !strings.Contains(ref, "..") {
			paths = append(paths, filepath.ToSlash(filepath.Join(".git", ref)))
		}
	}
	entries := make([]Entry, 0, len(paths))
	for _, rel := range paths {
		absolute := filepath.Join(grant.CanonicalRoot, filepath.FromSlash(rel))
		resolved, resolveErr := filepath.EvalSymlinks(absolute)
		if resolveErr != nil || !withinRoot(grant.CanonicalRoot, resolved) {
			continue
		}
		data, readErr := os.ReadFile(resolved)
		if readErr != nil || int64(len(data)) > s.options.MaxFileBytes || isBinary(data) {
			continue
		}
		content := redactCredentials(string(data))
		entries = append(entries, Entry{Path: rel, Kind: "git_metadata", Size: int64(len(data)), SHA256: fileChecksum(data), StartLine: 1, EndLine: lineCount(data), Content: content, Summary: content})
	}
	return entries, nil
}

func (s *Service) readAllowedFile(grant Grant, requestedPath string) ([]byte, os.FileInfo, error) {
	rel, err := s.relativePath(grant, requestedPath)
	if err != nil {
		return nil, nil, err
	}
	if shouldIgnore(rel, false, loadIgnoreRules(grant.CanonicalRoot)) {
		return nil, nil, fmt.Errorf("%w: file is ignored by repository policy", domainInvalid())
	}
	resolved, err := s.resolvePath(grant, rel)
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%w: path must identify a regular file", domainInvalid())
	}
	if info.Size() > s.options.MaxFileBytes {
		return nil, nil, fmt.Errorf("%w: file exceeds the configured size limit", domainInvalid())
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, nil, fmt.Errorf("read repository file: %w", err)
	}
	if isBinary(data) {
		return nil, nil, fmt.Errorf("%w: binary files are not readable", domainInvalid())
	}
	return data, info, nil
}

func (s *Service) resolvePath(grant Grant, requestedPath string) (string, error) {
	requestedPath = strings.TrimSpace(requestedPath)
	if requestedPath == "" {
		requestedPath = "."
	}
	if filepath.IsAbs(requestedPath) || strings.ContainsRune(requestedPath, '\x00') {
		return "", fmt.Errorf("%w: repository paths must be relative", domainInvalid())
	}
	clean := filepath.Clean(filepath.FromSlash(requestedPath))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: path traversal is not allowed", domainInvalid())
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(grant.CanonicalRoot, clean))
	if err != nil {
		return "", fmt.Errorf("%w: repository path does not exist", domainInvalid())
	}
	if !withinRoot(grant.CanonicalRoot, resolved) {
		return "", fmt.Errorf("%w: repository path escapes the authorized root", domainInvalid())
	}
	return resolved, nil
}

func (s *Service) relativePath(grant Grant, requestedPath string) (string, error) {
	resolved, err := s.resolvePath(grant, requestedPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(grant.CanonicalRoot, resolved)
	if err != nil || rel == "." {
		if rel == "." {
			return rel, nil
		}
		return "", fmt.Errorf("%w: resolve repository-relative path", domainInvalid())
	}
	return filepath.ToSlash(rel), nil
}

func withinRoot(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func loadIgnoreRules(root string) []ignoreRule {
	rules := []ignoreRule{}
	for _, name := range []string{".gitignore", filepath.Join(".git", "info", "exclude")} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || len(data) > 256<<10 {
			continue
		}
		for _, raw := range strings.Split(string(data), "\n") {
			line := strings.TrimSpace(raw)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			rule := ignoreRule{negate: strings.HasPrefix(line, "!")}
			line = strings.TrimPrefix(line, "!")
			rule.dirOnly = strings.HasSuffix(line, "/")
			rule.pattern = strings.Trim(strings.TrimSuffix(line, "/"), "/")
			if rule.pattern != "" {
				rules = append(rules, rule)
			}
		}
	}
	return rules
}

func shouldIgnore(rel string, directory bool, rules []ignoreRule) bool {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	if rel == ".git" || strings.HasPrefix(rel, ".git/") || sensitivePath(rel) || (!directory && binaryPath(rel)) {
		return true
	}
	ignored := false
	for _, rule := range rules {
		if rule.dirOnly && !directory && !strings.HasPrefix(rel, rule.pattern+"/") {
			continue
		}
		matched := false
		if strings.Contains(rule.pattern, "/") {
			matched, _ = path.Match(rule.pattern, rel)
			matched = matched || strings.HasPrefix(rel, rule.pattern+"/")
		} else {
			for _, component := range strings.Split(rel, "/") {
				if ok, _ := path.Match(rule.pattern, component); ok {
					matched = true
					break
				}
			}
		}
		if matched {
			ignored = !rule.negate
		}
	}
	return ignored
}

func sensitivePath(rel string) bool {
	for _, component := range strings.Split(strings.ToLower(filepath.ToSlash(rel)), "/") {
		if component == ".aws" || component == ".gnupg" || component == ".ssh" || component == ".terraform" {
			return true
		}
	}
	base := strings.ToLower(path.Base(filepath.ToSlash(rel)))
	ext := strings.ToLower(filepath.Ext(base))
	if _, blocked := secretExtensions[ext]; blocked {
		return true
	}
	if base == ".env" || strings.HasPrefix(base, ".env.") || base == ".netrc" || base == ".npmrc" || base == ".pypirc" || base == "id_rsa" || base == "id_ed25519" {
		return true
	}
	return strings.Contains(base, "credentials") || strings.Contains(base, "secret")
}

func binaryPath(rel string) bool {
	_, blocked := binaryExtensions[strings.ToLower(filepath.Ext(rel))]
	return blocked
}

func isBinary(data []byte) bool {
	probe := data
	if len(probe) > 8192 {
		probe = probe[:8192]
	}
	return bytes.IndexByte(probe, 0) >= 0
}

func isTestFile(rel string) bool {
	base := strings.ToLower(path.Base(filepath.ToSlash(rel)))
	return strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, "_test.py") || strings.HasPrefix(base, "test_") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.")
}

func redactCredentials(value string) string {
	value = credentialValue.ReplaceAllString(value, "$1=[REDACTED]")
	value = bearerValue.ReplaceAllString(value, "Bearer [REDACTED]")
	value = urlCredential.ReplaceAllString(value, "$1[REDACTED]@")
	return knownTokenValue.ReplaceAllString(value, "[REDACTED]")
}

func fileChecksum(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func lineCount(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	return bytes.Count(data, []byte("\n")) + 1
}

func domainInvalid() error { return domain.ErrInvalid }
