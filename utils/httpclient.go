package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

func ForwardTo(req *http.Request, handle func(http.Header, io.Reader) error) error {
	resp, err := sendReq(req)
	if err != nil || resp == nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		rb, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("response has status:%s and body:%q", resp.Status, rb)
	}

	if handle != nil {
		return handle(resp.Header, resp.Body)
	}

	return nil
}

func sendReq(req *http.Request) (resp *http.Response, err error) {
	if resp, err = http.DefaultClient.Do(req); err == nil {
		return
	}

	maxRetries := 3
	backoff := 10 * time.Millisecond

	for retries := 1; retries < maxRetries; retries++ {
		time.Sleep(backoff)
		backoff *= 2

		if resp, err = http.DefaultClient.Do(req); err == nil {
			break
		}
	}
	return
}

func ReadOnce(r io.Reader, part string, buf []byte, checkLen bool) (int, error) {
	n, err := r.Read(buf)
	if err != nil && n == 0 {
		return n, fmt.Errorf("read %s, err: %v", part, err)
	}

	if checkLen && n != len(buf) {
		return n, fmt.Errorf(
			"encounter unexpect EOF for %s, expect to read %d bytes, but got %d",
			part, len(buf), n,
		)
	}

	return n, nil
}

func ReadData(r io.Reader, name string, total int64) ([]byte, error) {
	last := total
	buf := make([]byte, last)

	for start, pn := int64(0), int64(0); last > 0; {
		pn = 8192
		if last < pn {
			pn = last
		}

		n, err := ReadOnce(r, name, buf[start:start+pn], false)
		if err != nil {
			return nil, err
		}

		pn = int64(n)
		start += pn
		last -= pn
	}

	return buf, nil
}

func ReadTo(ctx context.Context, r io.Reader, buf []byte) (int, error) {
	last := len(buf)

	for start, n := 0, 0; last > 0; {
		if IsCtxDone(ctx) {
			return 0, fmt.Errorf("canceled")
		}

		if n = 8192; last < n {
			n = last
		}

		n, err := r.Read(buf[start : start+n])
		if err != nil && n == 0 {
			if errors.Is(err, io.EOF) {
				return start, nil
			}

			return start, err
		}

		start += n
		last -= n
	}

	return len(buf), nil
}

func Write(ctx context.Context, w io.Writer, data []byte) error {
	for offset, total := 0, len(data); offset < total; {
		if IsCtxDone(ctx) {
			return fmt.Errorf("canceled")
		}

		n, err := w.Write(data[offset:])
		if err != nil {
			return err
		}

		offset += n
	}

	return nil
}

func JsonMarshal(t interface{}) ([]byte, error) {
	buffer := &bytes.Buffer{}
	enc := json.NewEncoder(buffer)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(t); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func GenQueryURI(endpoint string, params map[string]string) (string, error) {
	v, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}

	if len(params) > 0 {
		q := v.Query()
		for k, v := range params {
			q.Add(k, v)
		}
		v.RawQuery = q.Encode()
	}

	return v.String(), nil
}

func GenURL(endpoint, query string) (string, error) {
	v, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}

	if query != "" {
		v.RawQuery = query
	}

	return v.String(), nil
}

func IsCtxDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
