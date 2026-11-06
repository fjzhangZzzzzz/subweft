//go:build !webui

package webui

import "net/http"

const Available = false

func Handler() http.Handler { return http.NotFoundHandler() }
