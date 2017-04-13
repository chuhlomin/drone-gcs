package main

import (
	"context"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	log "github.com/Sirupsen/logrus"
	"github.com/mattn/go-zglob"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

// Plugin defines the GCS plugin parameters.
type Plugin struct {
	Credentials string
	Bucket      string

	// Indicates the files ACL, which should be one
	// of the following:
	//     private
	//     public
	Access string

	// Copies the files from the specified directory.
	// Regexp matching will apply to match multiple
	// files
	//
	// Examples:
	//    /path/to/file
	//    /path/to/*.txt
	//    /path/to/*/*.txt
	//    /path/to/**
	Source string
	Target string

	// Strip the prefix from the target path
	StripPrefix string

	// Recursive uploads
	Recursive bool

	// Exclude files matching this pattern.
	Exclude []string

	// Dry run without uploading/
	DryRun bool
}

// Exec runs the plugin
func (p *Plugin) Exec() error {
	// normalize the target URL
	if strings.HasPrefix(p.Target, "/") {
		p.Target = p.Target[1:]
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create the config
	config, err := google.JWTConfigFromJSON([]byte(p.Credentials))
	if err != nil {
		return err
	}

	// create the storage client with the application credentials
	gcc, err := storage.NewClient(ctx, option.WithTokenSource(config.TokenSource(ctx)))
	if err != nil {
		return err
	}
	defer gcc.Close()

	// find the bucket
	log.WithFields(log.Fields{
		"bucket": p.Bucket,
	}).Info("Attempting to upload")

	// create the bucket handle
	bkt := gcc.Bucket(p.Bucket)

	matches, err := matches(p.Source, p.Exclude)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Could not match files")
		return err
	}

	for _, match := range matches {

		stat, err := os.Stat(match)
		if err != nil {
			continue // should never happen
		}

		// skip directories
		if stat.IsDir() {
			continue
		}

		target := filepath.Join(p.Target, strings.TrimPrefix(match, p.StripPrefix))
		if !strings.HasPrefix(target, "/") {
			target = "/" + target
		}

		if err := p.uploadFile(ctx, bkt, match, target); err != nil {
			log.WithFields(log.Fields{
				"name":   match,
				"bucket": p.Bucket,
				"target": target,
				"error":  err,
			}).Error("Could not upload file")
			return err
		}
	}

	return nil
}

// uploadFile performs the actual uploading process.
func (p *Plugin) uploadFile(ctx context.Context, bkt *storage.BucketHandle, match, target string) error {

	// gcp has pretty crappy default content-type headers so this pluign
	// attempts to provide a proper content-type.
	content := contentType(match)

	// log file for debug purposes.
	log.WithFields(log.Fields{
		"name":         match,
		"bucket":       p.Bucket,
		"target":       target,
		"content-type": content,
	}).Info("Uploading file")

	// when executing a dry-run we exit because we don't actually want to
	// upload the file to GCP.
	if p.DryRun {
		return nil
	}

	f, err := os.Open(match)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"file":  match,
		}).Error("Problem opening file")
		return err
	}
	defer f.Close()

	obj := bkt.Object(target)

	w := obj.NewWriter(ctx)
	if _, err := io.Copy(w, f); err != nil {
		return err
	}

	if err := w.Close(); err != nil {
		return err
	}

	if p.Access == "public" {
		if err := obj.ACL().Set(ctx, storage.AllUsers, storage.RoleReader); err != nil {
			return err
		}
	}

	_, err = obj.Update(ctx, storage.ObjectAttrsToUpdate{
		ContentType: content,
	})
	if err != nil {
		return err
	}

	return nil
}

// matches is a helper function that returns a list of all files matching the
// included Glob pattern, while excluding all files that matche the exclusion
// Glob pattners.
func matches(include string, exclude []string) ([]string, error) {
	matches, err := zglob.Glob(include)
	if err != nil {
		return nil, err
	}
	if len(exclude) == 0 {
		return matches, nil
	}

	// find all files that are excluded and load into a map. we can verify
	// each file in the list is not a member of the exclusion list.
	excludem := map[string]bool{}
	for _, pattern := range exclude {
		excludes, err := zglob.Glob(pattern)
		if err != nil {
			return nil, err
		}
		for _, match := range excludes {
			excludem[match] = true
		}
	}

	var included []string
	for _, include := range matches {
		_, ok := excludem[include]
		if ok {
			continue
		}
		included = append(included, include)
	}
	return included, nil
}

// contentType is a helper function that returns the content type for the file
// based on extension. If the file extension is unknown application/octet-stream
// is returned.
func contentType(path string) string {
	ext := filepath.Ext(path)
	typ := mime.TypeByExtension(ext)
	if typ == "" {
		typ = "application/octet-stream"
	}
	return typ
}
