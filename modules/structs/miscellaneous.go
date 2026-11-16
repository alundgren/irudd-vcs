// Copyright 2015 The Gogs Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package structs

// SearchResults results of a successful search
type SearchResults struct {
	// OK indicates if the search was successful
	OK bool `json:"ok"`
	// Data contains the repository search results
	Data []*Repository `json:"data"`
}

// SearchError error of a failed search
type SearchError struct {
	// OK indicates the search status (always false for errors)
	OK bool `json:"ok"`
	// Error contains the error message
	Error string `json:"error"`
}

// ServerVersion wraps the version of the server
type ServerVersion struct {
	// Version is the server version string
	Version string `json:"version"`
}

// APIError is an api error with a message
type APIError struct {
	// Message contains the error description
	Message string `json:"message"`
	// URL contains the documentation URL for this error
	URL string `json:"url"`
}
