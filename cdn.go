package txcos

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/url"
	"path/filepath"
	"strconv"
	"time"
)

type CDN struct {
	domain string
	key    string
}

func NewCDN(domain, key string) *CDN {
	return &CDN{
		domain: domain,
		key:    key,
	}
}

func (c *CDN) sign(filePath string) (values url.Values, err error) {
	var timestamp = strconv.FormatInt(time.Now().Unix(), 10)
	var hash = md5.New()
	hash.Write([]byte(c.key + filepath.Join("/", filePath) + timestamp))
	var signature = hex.EncodeToString(hash.Sum(nil))

	values = url.Values{}
	values.Set("sign", signature)
	values.Set("t", timestamp)

	return values, err
}

func (c *CDN) GetAuthValues(ctx context.Context, filePath string) (values url.Values, err error) {
	fileURL, err := url.Parse(filePath)
	if err != nil {
		return nil, err
	}

	values, err = c.sign(fileURL.EscapedPath())
	if err != nil {
		return nil, err
	}

	return values, err
}

func (c *CDN) GetAuthURL(ctx context.Context, filePath string) (authURL string, err error) {
	fileURL, err := url.Parse(filePath)
	if err != nil {
		return "", err
	}

	values, err := c.sign(fileURL.EscapedPath())
	if err != nil {
		return "", err
	}

	newURL, err := url.Parse(c.domain)
	if err != nil {
		return "", err
	}
	newURL.Path = fileURL.Path
	newURL.RawQuery = values.Encode()

	return newURL.String(), nil
}
