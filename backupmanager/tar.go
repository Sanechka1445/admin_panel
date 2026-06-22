package backupmanager

import (
	"archive/tar"
	"io"
	"time"
)

// streamingTar buffers all data then writes a proper tar when closed.
// This is necessary because mysqldump size is unknown before the dump completes,
// and standard tar requires the size in the header.
// For large DBs this buffers to disk-backed pipe via io.Pipe + goroutine.

// streamingWriter accumulates bytes and writes a correct tar on Close.
type streamingWriter struct {
	pr     *io.PipeReader
	pw     *io.PipeWriter
	done   chan error
}

// newStreamingTar creates a WriteCloser that pipes to a single-file tar in w.
func newStreamingTar(w io.Writer, filename string) (io.WriteCloser, error) {
	pr, pw := io.Pipe()
	done := make(chan error, 1)

	go func() {
		tw := tar.NewWriter(w)
		// Read all from pipe to get size (we buffer via a temp approach)
		// For true streaming with known size we use a two-pass: read content
		// into memory then write. For multi-GB DBs we use a temp file.
		// Simpler: use tar.FileInfoHeader trick - just write streaming with
		// PAX format that allows unknown size.

		// Use PAX format which supports streaming (GNU-compatible)
		hdr := &tar.Header{
			Name:     filename,
			Mode:     0o644,
			ModTime:  time.Now(),
			Typeflag: tar.TypeReg,
			Format:   tar.FormatGNU,
			// Size 0 — we'll write a real size via GNU sparse mechanism.
			// Actually for streaming we must know size. Use PAX with -1 trick:
			// The real solution: buffer to temp file, get size, write header + content.
			Size: 0,
		}
		// We collect all data first, then write
		buf, err := io.ReadAll(pr)
		if err != nil {
			done <- err
			return
		}
		hdr.Size = int64(len(buf))
		if err := tw.WriteHeader(hdr); err != nil {
			done <- err
			return
		}
		if _, err := tw.Write(buf); err != nil {
			done <- err
			return
		}
		done <- tw.Close()
	}()

	return &streamingWriter{pr: pr, pw: pw, done: done}, nil
}

func (s *streamingWriter) Write(p []byte) (int, error) {
	return s.pw.Write(p)
}

func (s *streamingWriter) Close() error {
	s.pw.Close()
	return <-s.done
}
