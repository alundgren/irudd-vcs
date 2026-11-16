// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gitea.dev/modules/log"
)

// Scheme describes protocol types
type Scheme string

// enumerates all the scheme types
const (
	HTTP     Scheme = "http"
	HTTPS    Scheme = "https" // only valid in ROOT_URL (TLS terminated by a reverse proxy), not as PROTOCOL
	FCGI     Scheme = "fcgi"
	FCGIUnix Scheme = "fcgi+unix"
	HTTPUnix Scheme = "http+unix"
)

const (
	PublicURLAuto   = "auto"
	PublicURLLegacy = "legacy"
	PublicURLNever  = "never"
)

// Server settings
var (
	// AppURL is the Application ROOT_URL. It always has a '/' suffix
	// It maps to ini:"ROOT_URL"
	AppURL string

	// PublicURLDetection controls how to use the HTTP request headers to detect public URL
	PublicURLDetection string

	// AppSubURL represents the sub-url mounting point for gitea, parsed from "ROOT_URL"
	// It is either "" or starts with '/' and ends without '/', such as '/{sub-path}'.
	// This value is empty if site does not have sub-url.
	AppSubURL string

	// UseSubURLPath makes Gitea handle requests with sub-path like "/sub-path/owner/repo/...",
	// to make it easier to debug sub-path related problems without a reverse proxy.
	UseSubURLPath bool

	// AppDataPath is the default path for storing data.
	// It maps to ini:"APP_DATA_PATH" in [server] and defaults to AppWorkPath + "/data"
	AppDataPath string

	// LocalURL is the url for locally running applications to contact Gitea. It always has a '/' suffix
	// It maps to ini:"LOCAL_ROOT_URL" in [server]
	LocalURL string

	// appTempPathInternal is the temporary path for the app, it is only an internal variable
	// DO NOT use it directly, always use AppDataTempDir
	appTempPathInternal string

	Protocol             Scheme
	Domain               string
	HTTPAddr             string
	HTTPPort             string
	StaticRootPath       string // builtin "options" assets (gitignore/label/readme templates) for non-bindata builds
	StaticCacheTime      time.Duration
	EnableGzip           bool
	UnixSocketPermission uint32
	EnablePprof          bool
	GracefulRestartable  bool
	GracefulHammerTime   time.Duration
	StartupTimeout       time.Duration
	PerWriteTimeout      = 30 * time.Second
	PerWritePerKbTimeout = 10 * time.Second
)

func loadServerFrom(rootCfg ConfigProvider) {
	sec := rootCfg.Section("server")
	AppName = rootCfg.Section("").Key("APP_NAME").MustString("Gitea: Git with a cup of tea")

	Domain = sec.Key("DOMAIN").MustString("localhost")
	HTTPAddr = sec.Key("HTTP_ADDR").MustString("0.0.0.0")
	HTTPPort = sec.Key("HTTP_PORT").MustString("3000")

	protocolCfg := sec.Key("PROTOCOL").String()
	switch protocolCfg {
	case "", "http":
		Protocol = HTTP
	case "fcgi":
		Protocol = FCGI
	case "fcgi+unix", "unix", "http+unix":
		switch protocolCfg {
		case "fcgi+unix":
			Protocol = FCGIUnix
		case "unix":
			log.Warn("unix PROTOCOL value is deprecated, please use http+unix")
			fallthrough
		default: // "http+unix"
			Protocol = HTTPUnix
		}
		UnixSocketPermissionRaw := sec.Key("UNIX_SOCKET_PERMISSION").MustString("666")
		UnixSocketPermissionParsed, err := strconv.ParseUint(UnixSocketPermissionRaw, 8, 32)
		if err != nil || UnixSocketPermissionParsed > 0o777 {
			log.Fatal("Failed to parse unixSocketPermission: %s", UnixSocketPermissionRaw)
		}

		UnixSocketPermission = uint32(UnixSocketPermissionParsed)
		if !filepath.IsAbs(HTTPAddr) {
			HTTPAddr = filepath.Join(AppWorkPath, HTTPAddr)
		}
	default:
		log.Fatal("Invalid PROTOCOL %q", protocolCfg)
	}
	GracefulRestartable = sec.Key("ALLOW_GRACEFUL_RESTARTS").MustBool(true)
	GracefulHammerTime = sec.Key("GRACEFUL_HAMMER_TIME").MustDuration(60 * time.Second)
	StartupTimeout = sec.Key("STARTUP_TIMEOUT").MustDuration(0 * time.Second)
	PerWriteTimeout = sec.Key("PER_WRITE_TIMEOUT").MustDuration(PerWriteTimeout)
	PerWritePerKbTimeout = sec.Key("PER_WRITE_PER_KB_TIMEOUT").MustDuration(PerWritePerKbTimeout)

	defaultAppURL := string(Protocol) + "://" + Domain + ":" + HTTPPort
	AppURL = sec.Key("ROOT_URL").MustString(defaultAppURL)
	PublicURLDetection = sec.Key("PUBLIC_URL_DETECTION").MustString(PublicURLAuto)
	if PublicURLDetection != PublicURLAuto && PublicURLDetection != PublicURLLegacy && PublicURLDetection != PublicURLNever {
		log.Fatal("Invalid PUBLIC_URL_DETECTION value: %s", PublicURLDetection)
	}

	// Check validity of AppURL
	appURL, err := url.Parse(AppURL)
	if err != nil {
		log.Fatal("Invalid ROOT_URL %q: %s", AppURL, err)
	}
	// Remove default ports from AppURL.
	// (scheme-based URL normalization, RFC 3986 section 6.2.3)
	if (appURL.Scheme == string(HTTP) && appURL.Port() == "80") || (appURL.Scheme == string(HTTPS) && appURL.Port() == "443") {
		appURL.Host = appURL.Hostname()
	}
	// This should be TrimRight to ensure that there is only a single '/' at the end of AppURL.
	AppURL = strings.TrimRight(appURL.String(), "/") + "/"

	// AppSubURL should start with '/' and end without '/', such as '/{subpath}'.
	// This value is empty if site does not have sub-url.
	AppSubURL = strings.TrimSuffix(appURL.Path, "/")
	UseSubURLPath = sec.Key("USE_SUB_URL_PATH").MustBool(false)

	// Check if Domain differs from AppURL domain than update it to AppURL's domain
	urlHostname := appURL.Hostname()
	if urlHostname != Domain && net.ParseIP(urlHostname) == nil && urlHostname != "" {
		Domain = urlHostname
	}

	var defaultLocalURL string
	switch Protocol {
	case HTTPUnix:
		defaultLocalURL = "http://unix/"
	case FCGI:
		defaultLocalURL = AppURL
	case FCGIUnix:
		defaultLocalURL = AppURL
	case HTTP:
		defaultLocalURL = string(Protocol) + "://"
		if HTTPAddr == "0.0.0.0" {
			defaultLocalURL += net.JoinHostPort("localhost", HTTPPort) + "/"
		} else {
			defaultLocalURL += net.JoinHostPort(HTTPAddr, HTTPPort) + "/"
		}
	default:
		log.Fatal("Invalid PROTOCOL %q", Protocol)
	}
	LocalURL = sec.Key("LOCAL_ROOT_URL").MustString(defaultLocalURL)
	LocalURL = strings.TrimRight(LocalURL, "/") + "/"
	if len(StaticRootPath) == 0 {
		StaticRootPath = AppWorkPath
	}
	StaticRootPath = sec.Key("STATIC_ROOT_PATH").MustString(StaticRootPath)
	StaticCacheTime = sec.Key("STATIC_CACHE_TIME").MustDuration(6 * time.Hour)
	AppDataPath = sec.Key("APP_DATA_PATH").MustString(filepath.Join(AppWorkPath, "data"))
	if !filepath.IsAbs(AppDataPath) {
		AppDataPath = filepath.ToSlash(filepath.Join(AppWorkPath, AppDataPath))
	}
	if IsInTesting && HasInstallLock(rootCfg) {
		// FIXME: in testing, the "app data" directory is not correctly initialized before loading settings
		if _, err := os.Stat(AppDataPath); err != nil {
			_ = os.MkdirAll(AppDataPath, os.ModePerm)
		}
	}

	appTempPathInternal = sec.Key("APP_TEMP_PATH").String()
	if appTempPathInternal != "" {
		if _, err := os.Stat(appTempPathInternal); err != nil {
			log.Fatal("APP_TEMP_PATH %q is not accessible: %v", appTempPathInternal, err)
		}
	}

	// TODO: GOLANG-HTTP-TMPDIR: Some Golang packages (like "http") use os.TempDir() to create temporary files when uploading files.
	// So ideally we should set the TMPDIR environment variable to make them use our managed temp directory.
	// But there is no clear place to set it currently, for example: when running "install" page, the AppDataPath is not ready yet, then AppDataTempDir won't work

	EnableGzip = sec.Key("ENABLE_GZIP").MustBool()
	EnablePprof = sec.Key("ENABLE_PPROF").MustBool(false)
}
