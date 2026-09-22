// Package releasearchive writes canonical Change Saga release archives.
package releasearchive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type entry struct {
	name string
	path string
	mode int64
}

// Write creates a deterministic release archive from a staged release tree.
func Write(output string, epoch int64, stage, binary string) (err error) {
	entries := []entry{
		{name: binary, path: filepath.Join(stage, binary), mode: 0o755},
		{name: "LICENSE", path: filepath.Join(stage, "LICENSE"), mode: 0o644},
		{name: "README.md", path: filepath.Join(stage, "README.md"), mode: 0o644},
	}

	temp, err := os.CreateTemp(filepath.Dir(output), ".release-archive-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() {
		temp.Close()
		if err != nil {
			os.Remove(tempName)
		}
	}()
	if err = temp.Chmod(0o644); err != nil {
		return err
	}

	stamp := time.Unix(epoch, 0).UTC()
	switch {
	case strings.HasSuffix(output, ".tar.gz"):
		err = writeTarGzip(temp, stamp, entries)
	case strings.HasSuffix(output, ".zip"):
		err = writeZip(temp, stamp, entries)
	default:
		err = fmt.Errorf("unsupported release archive name %q", output)
	}
	if err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, output)
}

// WriteTreeZip creates a deterministic ZIP containing the regular files below
// source under one top-level archive directory. Files are sorted, timestamps
// are pinned to epoch, and modes are normalized so the result does not depend
// on the builder. Links and other special files are rejected rather than
// publishing an archive whose extraction could escape or vary by platform.
func WriteTreeZip(output string, epoch int64, source, archiveRoot string) (err error) {
	if path.Clean(archiveRoot) != archiveRoot || archiveRoot == "." || archiveRoot == ".." ||
		strings.Contains(archiveRoot, "/") || strings.Contains(archiveRoot, `\`) || !safeArchiveRoot(archiveRoot) {
		return fmt.Errorf("unsafe archive root %q", archiveRoot)
	}

	var entries []entry
	err = filepath.WalkDir(source, func(filePath string, item os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, filePath)
		if err != nil {
			return err
		}
		if relative == "." {
			if item.Type()&os.ModeSymlink != 0 || !item.IsDir() {
				return fmt.Errorf("release tree root %q is not a directory", source)
			}
			return nil
		}
		if item.IsDir() {
			return nil
		}
		if item.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("release tree input %q is a symbolic link", filePath)
		}
		info, err := item.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("release tree input %q is not a regular file", filePath)
		}
		relative = filepath.ToSlash(relative)
		if !safeArchiveRelative(relative) {
			return fmt.Errorf("release tree input %q has a non-portable archive path", filePath)
		}
		name := path.Join(archiveRoot, relative)
		if !strings.HasPrefix(name, archiveRoot+"/") {
			return fmt.Errorf("release tree input %q escapes archive root", filePath)
		}
		entries = append(entries, entry{name: name, path: filePath, mode: 0o644})
		return nil
	})
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("release tree %q contains no regular files", source)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })

	temp, err := os.CreateTemp(filepath.Dir(output), ".release-tree-archive-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() {
		temp.Close()
		if err != nil {
			os.Remove(tempName)
		}
	}()
	if err = temp.Chmod(0o644); err != nil {
		return err
	}
	if err = writeZip(temp, time.Unix(epoch, 0).UTC(), entries); err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, output)
}

func safeArchiveRoot(name string) bool {
	for index, char := range []byte(name) {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			continue
		}
		if index > 0 && (char == '.' || char == '_' || char == '-') {
			continue
		}
		return false
	}
	return name != ""
}

func safeArchiveRelative(name string) bool {
	if !utf8.ValidString(name) || path.Clean(name) != name || path.IsAbs(name) ||
		strings.Contains(name, `\`) || strings.ContainsAny(name, `:*?<>|"`) {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return strings.IndexFunc(name, unicode.IsControl) < 0
}

func writeTarGzip(output io.Writer, stamp time.Time, entries []entry) error {
	gz, err := gzip.NewWriterLevel(output, gzip.BestCompression)
	if err != nil {
		return err
	}
	gz.Header.ModTime = stamp
	gz.Header.OS = 255
	tw := tar.NewWriter(gz)
	for _, item := range entries {
		if err := writeTarEntry(tw, stamp, item); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func writeTarEntry(tw *tar.Writer, stamp time.Time, item entry) error {
	file, size, err := openEntry(item.path)
	if err != nil {
		return err
	}
	defer file.Close()
	header := &tar.Header{
		Name: item.name, Mode: item.mode, Size: size, ModTime: stamp,
		Typeflag: tar.TypeReg, Format: tar.FormatUSTAR,
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err = io.CopyN(tw, file, size)
	return err
}

func writeZip(output io.Writer, stamp time.Time, entries []entry) error {
	zw := zip.NewWriter(output)
	for _, item := range entries {
		file, size, err := openEntry(item.path)
		if err != nil {
			return err
		}
		header := &zip.FileHeader{Name: item.name, Method: zip.Deflate}
		header.SetModTime(stamp)
		header.SetMode(os.FileMode(item.mode))
		writer, err := zw.CreateHeader(header)
		if err == nil {
			_, err = io.CopyN(writer, file, size)
		}
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return zw.Close()
}

func openEntry(path string) (*os.File, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, 0, fmt.Errorf("release input %q is not a regular file", path)
	}
	return file, info.Size(), nil
}
