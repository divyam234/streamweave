package nzbdav

import (
	"errors"
	"net/http"
	"strings"

	"streamweave/internal/resolver/sabdav"
)

func NewResolver(id, credential string, client *http.Client) (*sabdav.Resolver, error) {
	var cfg sabdav.Credential
	if err := sabdav.ParseCredential(credential, &cfg); err != nil {
		return nil, err
	}
	cfg.NzbDavURL = strings.TrimRight(strings.TrimSpace(cfg.NzbDavURL), "/")
	if cfg.NzbDavURL == "" || strings.TrimSpace(cfg.NzbDavAPIKey) == "" {
		return nil, errors.New("NzbDAV credential requires nzbdavUrl and nzbdavApiKey")
	}
	publicURL := strings.TrimRight(strings.TrimSpace(cfg.PublicNzbDavURL), "/")
	if publicURL == "" {
		publicURL = cfg.NzbDavURL
	}
	return sabdav.New(id, sabdav.Config{
		Service:        "NzbDAV",
		BaseURL:        cfg.NzbDavURL,
		PublicURL:      publicURL,
		APIURL:         cfg.NzbDavURL + "/api",
		APIKey:         cfg.NzbDavAPIKey,
		WebDAVUser:     cfg.WebDAVUser,
		WebDAVPassword: cfg.WebDAVPassword,
		ContentPrefix:  "/content",
	}, client)
}
