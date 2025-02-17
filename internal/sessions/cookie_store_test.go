package sessions // import "github.com/pomerium/pomerium/internal/sessions"

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pomerium/pomerium/internal/cryptutil"
	"github.com/pomerium/pomerium/internal/encoding"
	"github.com/pomerium/pomerium/internal/encoding/ecjson"
	"github.com/pomerium/pomerium/internal/encoding/mock"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestNewCookieStore(t *testing.T) {
	cipher, err := cryptutil.NewAEADCipher(cryptutil.NewKey())
	if err != nil {
		t.Fatal(err)
	}
	encoder := ecjson.New(cipher)
	tests := []struct {
		name    string
		opts    *CookieOptions
		encoder encoding.MarshalUnmarshaler
		want    *CookieStore
		wantErr bool
	}{
		{"good", &CookieOptions{Name: "_cookie", Secure: true, HTTPOnly: true, Domain: "pomerium.io", Expire: 10 * time.Second}, encoder, &CookieStore{Name: "_cookie", Secure: true, HTTPOnly: true, Domain: "pomerium.io", Expire: 10 * time.Second}, false},
		{"missing name", &CookieOptions{Name: "", Secure: true, HTTPOnly: true, Domain: "pomerium.io", Expire: 10 * time.Second}, encoder, nil, true},
		{"missing encoder", &CookieOptions{Name: "_cookie", Secure: true, HTTPOnly: true, Domain: "pomerium.io", Expire: 10 * time.Second}, nil, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewCookieStore(tt.opts, tt.encoder)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCookieStore() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			cmpOpts := []cmp.Option{
				cmpopts.IgnoreUnexported(CookieStore{}),
			}

			if diff := cmp.Diff(got, tt.want, cmpOpts...); diff != "" {
				t.Errorf("NewCookieStore() = %s", diff)
			}
		})
	}
}
func TestNewCookieLoader(t *testing.T) {
	cipher, err := cryptutil.NewAEADCipher(cryptutil.NewKey())
	if err != nil {
		t.Fatal(err)
	}
	encoder := ecjson.New(cipher)
	tests := []struct {
		name    string
		opts    *CookieOptions
		encoder encoding.MarshalUnmarshaler
		want    *CookieStore
		wantErr bool
	}{
		{"good", &CookieOptions{Name: "_cookie", Secure: true, HTTPOnly: true, Domain: "pomerium.io", Expire: 10 * time.Second}, encoder, &CookieStore{Name: "_cookie", Secure: true, HTTPOnly: true, Domain: "pomerium.io", Expire: 10 * time.Second}, false},
		{"missing name", &CookieOptions{Name: "", Secure: true, HTTPOnly: true, Domain: "pomerium.io", Expire: 10 * time.Second}, encoder, nil, true},
		{"missing encoder", &CookieOptions{Name: "_cookie", Secure: true, HTTPOnly: true, Domain: "pomerium.io", Expire: 10 * time.Second}, nil, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewCookieLoader(tt.opts, tt.encoder)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCookieLoader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			cmpOpts := []cmp.Option{
				cmpopts.IgnoreUnexported(CookieStore{}),
			}

			if diff := cmp.Diff(got, tt.want, cmpOpts...); diff != "" {
				t.Errorf("NewCookieLoader() = %s", diff)
			}
		})
	}
}

func TestCookieStore_SaveSession(t *testing.T) {
	c, err := cryptutil.NewAEADCipher(cryptutil.NewKey())
	if err != nil {
		t.Fatal(err)
	}

	hugeString := make([]byte, 4097)
	if _, err := rand.Read(hugeString); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		// State       *State
		State       interface{}
		encoder     encoding.Marshaler
		decoder     encoding.Unmarshaler
		wantErr     bool
		wantLoadErr bool
	}{
		{"good", &State{Email: "user@domain.com", User: "user"}, ecjson.New(c), ecjson.New(c), false, false},
		{"bad cipher", &State{Email: "user@domain.com", User: "user"}, nil, nil, true, true},
		{"huge cookie", &State{Subject: fmt.Sprintf("%x", hugeString), Email: "user@domain.com", User: "user"}, ecjson.New(c), ecjson.New(c), false, false},
		{"marshal error", &State{Email: "user@domain.com", User: "user"}, mock.Encoder{MarshalError: errors.New("error")}, ecjson.New(c), true, true},
		{"nil encoder cannot save non string type", &State{Email: "user@domain.com", User: "user"}, nil, ecjson.New(c), true, true},
		{"good marshal string directly", cryptutil.NewBase64Key(), nil, ecjson.New(c), false, true},
		{"good marshal bytes directly", cryptutil.NewKey(), nil, ecjson.New(c), false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &CookieStore{
				Name:     "_pomerium",
				Secure:   true,
				HTTPOnly: true,
				Domain:   "pomerium.io",
				Expire:   10 * time.Second,
				encoder:  tt.encoder,
				decoder:  tt.decoder,
			}

			r := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()

			if err := s.SaveSession(w, r, tt.State); (err != nil) != tt.wantErr {
				t.Errorf("CookieStore.SaveSession() error = %v, wantErr %v", err, tt.wantErr)
			}
			r = httptest.NewRequest("GET", "/", nil)
			for _, cookie := range w.Result().Cookies() {
				r.AddCookie(cookie)
			}

			state, err := s.LoadSession(r)
			if (err != nil) != tt.wantLoadErr {
				t.Errorf("LoadSession() error = %v, wantErr %v", err, tt.wantLoadErr)
				return
			}
			cmpOpts := []cmp.Option{
				cmpopts.IgnoreUnexported(State{}),
			}
			if err == nil {
				if diff := cmp.Diff(state, tt.State, cmpOpts...); diff != "" {
					t.Errorf("CookieStore.LoadSession() got = %s", diff)
				}
			}
			w = httptest.NewRecorder()
			s.ClearSession(w, r)
			x := w.Header().Get("Set-Cookie")
			if !strings.Contains(x, "_pomerium=; Path=/;") {
				t.Errorf(x)
			}
		})
	}
}
