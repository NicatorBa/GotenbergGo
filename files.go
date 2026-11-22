package gotenberg

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
)

// file is a multipart file part: a reference to a path or an fs.FS entry,
// opened when the request is sent, or content held in memory.
type file struct {
	field    string
	filename string
	// fixed marks a file uploaded under a name the route requires, such as
	// index.html: it is never renamed for merge order.
	fixed bool

	path string
	fsys fs.FS
	name string
	data []byte
}

// open returns the reader of a file reference, or nil for in-memory content.
func (f file) open() (io.ReadCloser, error) {
	switch {
	case f.fsys != nil:
		rc, err := f.fsys.Open(f.name)
		if err != nil {
			return nil, errorf("open %s: %w", f.name, err)
		}

		return rc, nil
	case f.path != "":
		rc, err := os.Open(f.path)
		if err != nil {
			return nil, errorf("open %s: %w", f.path, err)
		}

		return rc, nil
	default:
		return nil, nil
	}
}

// write copies the content of the file to w; rc is the reader open returned.
func (f file) write(w io.Writer, rc io.ReadCloser) error {
	if rc == nil {
		_, err := w.Write(f.data)
		return err
	}

	_, err := io.Copy(w, rc)
	if err != nil {
		return fmt.Errorf("read %s: %w", f.filename, err)
	}

	return nil
}

// openFiles opens every file reference before the request starts. On error,
// the files opened so far are closed.
func openFiles(files []file) ([]io.ReadCloser, error) {
	readers := make([]io.ReadCloser, len(files))

	for i, f := range files {
		rc, err := f.open()
		if err != nil {
			closeAll(readers)
			return nil, err
		}

		readers[i] = rc
	}

	return readers, nil
}

func closeAll(readers []io.ReadCloser) {
	for _, rc := range readers {
		if rc != nil {
			_ = rc.Close()
		}
	}
}

// addFile appends f, copying the files first.
func (r *request) addFile(f file) {
	r.files = append(slices.Clip(r.files), f)
}

// replaceFile adds f, replacing a file previously added under the same field
// and fixed filename.
func (r *request) replaceFile(f file) {
	r.files = slices.DeleteFunc(slices.Clone(r.files), func(other file) bool {
		return other.fixed && other.field == f.field && other.filename == f.filename
	})
	r.files = append(r.files, f)
}

func (r *request) fileFromPath(field, p, filename string) {
	switch {
	case p == "":
		r.addErr(errors.New("file from path: empty path"))
	case filename == "" || filename == "." || filename == string(filepath.Separator):
		r.addErr(fmt.Errorf("file from path %q: empty filename", p))
	default:
		r.addFile(file{field: field, filename: filename, path: p})
	}
}

func (r *request) fileFromFS(field string, fsys fs.FS, name, filename string) {
	switch {
	case fsys == nil:
		r.addErr(fmt.Errorf("file from fs %q: nil fs.FS", name))
	case !fs.ValidPath(name) || name == ".":
		r.addErr(fmt.Errorf("file from fs: invalid name %q", name))
	case filename == "":
		r.addErr(fmt.Errorf("file from fs %q: empty filename", name))
	default:
		r.addFile(file{field: field, filename: filename, fsys: fsys, name: name})
	}
}

func (r *request) fileFromReader(field, filename string, reader io.Reader) {
	data, err := readAll(filename, reader)
	if err != nil {
		r.addErr(err)
		return
	}

	r.fileFromBytes(field, filename, data)
}

func (r *request) fileFromBytes(field, filename string, data []byte) {
	if filename == "" {
		r.addErr(errors.New("file from bytes: empty filename"))
		return
	}

	r.addFile(file{field: field, filename: filename, data: data})
}

// FileFrom… helpers of the generated builders.

func (r *request) addFileFromPath(p string) {
	r.fileFromPath(filesField, p, filepath.Base(p))
}

func (r *request) addFileFromPathAs(p, filename string) {
	r.fileFromPath(filesField, p, filename)
}

func (r *request) addFileFromFS(fsys fs.FS, name string) {
	r.fileFromFS(filesField, fsys, name, path.Base(name))
}

func (r *request) addFileFromFSAs(fsys fs.FS, name, filename string) {
	r.fileFromFS(filesField, fsys, name, filename)
}

func (r *request) addFileFromReader(filename string, reader io.Reader) {
	r.fileFromReader(filesField, filename, reader)
}

func (r *request) addFileFromBytes(filename string, data []byte) {
	r.fileFromBytes(filesField, filename, data)
}

// ArchiveFrom… helpers of the generated builders.

func (r *request) addArchiveFromPath(p string) {
	r.fileFromPath(archivesField, p, filepath.Base(p))
}

func (r *request) addArchiveFromFS(fsys fs.FS, name string) {
	r.fileFromFS(archivesField, fsys, name, path.Base(name))
}

func (r *request) addArchiveFromReader(reader io.Reader) {
	r.fileFromReader(archivesField, r.archiveName(), reader)
}

func (r *request) addArchiveFromBytes(data []byte) {
	r.fileFromBytes(archivesField, r.archiveName(), data)
}

// archiveName names an archive added from memory, whose own name is
// irrelevant to the server: archive1.zip, archive2.zip, …
func (r *request) archiveName() string {
	n := 1
	for _, f := range r.files {
		if f.field == archivesField {
			n++
		}
	}

	return "archive" + strconv.Itoa(n) + ".zip"
}

// Helpers of the fileContent fields, such as IndexFromPath: the file is
// uploaded under files with the fixed filename, replacing a previous one.

func (r *request) contentFromPath(filename, p string) {
	if p == "" {
		r.addErr(fmt.Errorf("%s from path: empty path", filename))
		return
	}

	r.replaceFile(file{field: filesField, filename: filename, fixed: true, path: p})
}

func (r *request) contentFromFS(filename string, fsys fs.FS, name string) {
	switch {
	case fsys == nil:
		r.addErr(fmt.Errorf("%s from fs %q: nil fs.FS", filename, name))
	case !fs.ValidPath(name) || name == ".":
		r.addErr(fmt.Errorf("%s from fs: invalid name %q", filename, name))
	default:
		r.replaceFile(file{field: filesField, filename: filename, fixed: true, fsys: fsys, name: name})
	}
}

func (r *request) contentFromReader(filename string, reader io.Reader) {
	data, err := readAll(filename, reader)
	if err != nil {
		r.addErr(err)
		return
	}

	r.contentFromBytes(filename, data)
}

func (r *request) contentFromBytes(filename string, data []byte) {
	r.replaceFile(file{field: filesField, filename: filename, fixed: true, data: data})
}

// readAll reads reader completely, so that the request does not depend on it
// when it is sent.
func readAll(filename string, reader io.Reader) ([]byte, error) {
	if reader == nil {
		return nil, fmt.Errorf("file from reader %q: nil reader", filename)
	}

	var buf bytes.Buffer
	_, err := buf.ReadFrom(reader)
	if err != nil {
		return nil, fmt.Errorf("file from reader %q: %w", filename, err)
	}

	return buf.Bytes(), nil
}

// uploads returns the files to send. When the request merges its input
// documents, which the server orders by filename, every input document added
// via FileFrom… is prefixed with its zero-padded position, such as
// 0001_a.pdf, so that the merge follows the order the files were added in.
// Files a stage references by filename are not renamed.
func (r request) uploads() []file {
	if !r.inputs || !(r.merge || slices.ContainsFunc(r.stages, isMergeStage)) {
		return r.files
	}

	referenced := map[string]bool{}
	for _, s := range r.stages {
		for _, names := range s.stageState().refs {
			for _, name := range names {
				referenced[name] = true
			}
		}
	}

	rename := func(f file) bool {
		return f.field == filesField && !f.fixed && !referenced[f.filename]
	}

	count := 0
	for _, f := range r.files {
		if rename(f) {
			count++
		}
	}

	width := max(4, len(strconv.Itoa(count)))

	files := slices.Clone(r.files)
	n := 0
	for i, f := range files {
		if rename(f) {
			n++
			files[i].filename = fmt.Sprintf("%0*d_%s", width, n, f.filename)
		}
	}

	return files
}

func isMergeStage(s Stage) bool {
	return s != nil && s.stageState().op == "merge"
}
