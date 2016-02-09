// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package email

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
)

// NewMessage ...
func NewMessage(r io.Reader) (*Message, error) {
	msg, err := mail.ReadMessage(&leftTrimReader{r: bufioReader(r)})
	if err != nil {
		return nil, err
	}
	return NewMessageWithHeader(Header(msg.Header), msg.Body)
}

// NewMessageWithHeader ...
func NewMessageWithHeader(headers Header, bodyReader io.Reader) (*Message, error) {

	if headers.Get("Content-Transfer-Encoding") == "quoted-printable" {
		headers.Del("Content-Transfer-Encoding")
		bodyReader = quotedprintable.NewReader(bodyReader)
	}

	var err error
	var mediaType string
	var subMessage *Message
	mediaTypeParams := make(map[string]string)
	preamble := make([]byte, 0, 0)
	epilogue := make([]byte, 0, 0)
	body := make([]byte, 0, 0)
	parts := make([]*Message, 0, 0)

	if contentType := headers.Get("Content-Type"); len(contentType) > 0 {
		mediaType, mediaTypeParams, err = mime.ParseMediaType(contentType)
		if err != nil {
			return nil, err
		}
	} // Lack of contentType is not a problem

	// Can only have one of the following: Parts, SubMessage, or Body
	if strings.HasPrefix(mediaType, "multipart") {
		boundary := mediaTypeParams["boundary"]
		preamble, err = readPreamble(bodyReader, boundary)
		if err == nil {
			parts, err = readParts(mediaType, mediaTypeParams, bodyReader, boundary)
			if err == nil {
				epilogue, err = ioutil.ReadAll(bodyReader)
			}
		}

	} else if strings.HasPrefix(mediaType, "message") {
		subMessage, err = NewMessage(bodyReader)

	} else {
		body, err = ioutil.ReadAll(bodyReader)
	}
	if err != nil {
		return nil, err
	}

	return &Message{
		Header:     headers,
		Preamble:   preamble,
		Epilogue:   epilogue,
		Body:       body,
		SubMessage: subMessage,
		Parts:      parts,
	}, nil
}

// readParts ...
func readParts(messageMedia string, messageMediaParams map[string]string, bodyReader io.Reader, boundary string) ([]*Message, error) {

	parts := make([]*Message, 0, 1)
	multipartReader := multipart.NewReader(bodyReader, boundary)

	for part, partErr := multipartReader.NextPart(); partErr != io.EOF; part, partErr = multipartReader.NextPart() {
		if partErr != nil && partErr != io.EOF {
			return []*Message{}, partErr
		}
		newEmailPart, msgErr := NewMessageWithHeader(Header(part.Header), part)
		part.Close()
		if msgErr != nil {
			return []*Message{}, msgErr
		}
		parts = append(parts, newEmailPart)
	}
	return parts, nil
}

// readPreamble ...
func readPreamble(r io.Reader, boundary string) ([]byte, error) {
	return ioutil.ReadAll(&preambleReader{r: bufioReader(r), boundary: []byte("--" + boundary)})
}

// preambleReader ...
type preambleReader struct {
	r        *bufio.Reader
	boundary []byte
}

// Read ...
func (r *preambleReader) Read(p []byte) (int, error) {
	// Peek and read up to the --boundary, then EOF
	peek, err := r.r.Peek(4096)
	if err != nil && err != io.EOF {
		return 0, fmt.Errorf("Preamble Read: %v", err)
	}

	idx := bytes.Index(peek, r.boundary)

	if idx < 0 {
		return r.r.Read(p)
	}

	// Account for possible new line at start of the boundary, which we shouldn't remove
	if idx <= 2 {
		return 0, io.EOF
	}

	preamble := make([]byte, idx-2)
	n, err := r.r.Read(preamble)
	copy(p, preamble)
	if err != nil && err != io.EOF {
		return n, err
	}
	return n, io.EOF
}
