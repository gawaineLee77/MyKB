// Package branding owns installation-wide presentation settings.
package branding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/enterprise"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/safeimage"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

const Route = "/api/v1/mindcreek/branding"
const DefaultName = "小纳"

type Error struct {
	Status int
	Code   string
}

func (e *Error) Error() string      { return e.Code }
func (e *Error) HTTPStatus() int    { return e.Status }
func (e *Error) PublicCode() string { return e.Code }

var ErrConflict = errors.New("branding.revision_conflict")

type Record struct {
	Name     string
	Logo     []byte
	Revision int64
}
type Snapshot struct {
	Name     string `json:"name"`
	LogoURL  string `json:"logo_url"`
	Revision int64  `json:"revision"`
}

func (r Record) Snapshot() Snapshot {
	s := Snapshot{Name: r.Name, Revision: r.Revision}
	if len(r.Logo) != 0 {
		s.LogoURL = fmt.Sprintf("%s/logo?v=%d", Route, r.Revision)
	}
	return s
}

type Input struct {
	Name     string `json:"name"`
	Revision int64  `json:"revision"`
	// Absent preserves the image; JSON null restores the bundled logo.
	LogoData json.RawMessage `json:"logo_data,omitempty"`
}
type Change struct {
	Name        string
	Revision    int64
	ReplaceLogo bool
	Logo        []byte
}
type Store interface {
	Get(context.Context) (Record, error)
	Update(context.Context, Change, string, string) (Record, error)
}
type InstallationStore interface {
	Installation(context.Context) (enterprise.Installation, error)
}
type Service struct {
	Store         Store
	Installations InstallationStore
}

func (s *Service) Get(ctx context.Context) (Record, error) {
	if s == nil || s.Store == nil {
		return Record{}, &Error{503, "branding.unavailable"}
	}
	r, err := s.Store.Get(ctx)
	if err != nil {
		return Record{}, &Error{503, "branding.unavailable"}
	}
	return r, nil
}

func (s *Service) CanManage(ctx context.Context, p weknora.Principal) error {
	if s == nil || s.Installations == nil {
		return &Error{503, "branding.unavailable"}
	}
	i, err := s.Installations.Installation(ctx)
	if err != nil {
		return &Error{503, "branding.unavailable"}
	}
	if !i.IsAdmin(p) {
		return &Error{403, "branding.installation_admin_required"}
	}
	return nil
}

func (s *Service) Update(ctx context.Context, p weknora.Principal, input Input, requestID string) (Record, error) {
	if err := s.CanManage(ctx, p); err != nil {
		return Record{}, err
	}
	name := strings.TrimSpace(input.Name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) == 0 || utf8.RuneCountInString(name) > 64 || input.Revision < 1 {
		return Record{}, &Error{400, "branding.input_invalid"}
	}
	for _, c := range name {
		if unicode.IsControl(c) || unicode.Is(unicode.Cf, c) || c == '<' || c == '>' {
			return Record{}, &Error{400, "branding.input_invalid"}
		}
	}
	change := Change{Name: name, Revision: input.Revision, ReplaceLogo: len(input.LogoData) > 0}
	if change.ReplaceLogo && string(input.LogoData) != "null" {
		var data string
		if json.Unmarshal(input.LogoData, &data) != nil {
			return Record{}, &Error{400, "branding.image_invalid"}
		}
		var err error
		change.Logo, err = safeimage.DataURL(data)
		if err != nil {
			return Record{}, &Error{400, "branding.image_invalid"}
		}
	}
	if s.Store == nil {
		return Record{}, &Error{503, "branding.unavailable"}
	}
	r, err := s.Store.Update(ctx, change, p.User.ID, requestID)
	if errors.Is(err, ErrConflict) {
		return Record{}, &Error{409, "branding.revision_conflict"}
	}
	if err != nil {
		return Record{}, &Error{503, "branding.unavailable"}
	}
	return r, nil
}
