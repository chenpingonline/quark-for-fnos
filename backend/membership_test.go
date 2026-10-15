package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type membershipTransport func(*http.Request) (*http.Response, error)

func (f membershipTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMembershipUsesAuthenticatedMemberEndpointAndFiltersFields(t *testing.T) {
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.setCredential("__puus=test-secret"); err != nil {
		t.Fatal(err)
	}
	q := newQuark(store)
	q.client.Transport = membershipTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() == "" || r.URL.Host != "drive-pc.quark.cn" || r.URL.Path != "/1/clouddrive/member" || r.URL.Query().Get("pr") != "ucpro" || r.URL.Query().Get("fr") != "pc" {
			t.Errorf("unexpected endpoint %s", r.URL)
		}
		if r.Header.Get("Cookie") != "__puus=test-secret" {
			t.Error("missing session")
		}
		if _, ok := r.Context().Deadline(); !ok {
			t.Error("missing bounded timeout")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":200,"code":0,"data":{"member_type":"SUPER_VIP","use_capacity":0,"total_capacity":6597069766656,"token":"DO_NOT_RETURN","member_status":{"SUPER_VIP":"unconfirmed-expiry"}}}`))}, nil
	})
	got, err := q.membership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "SUPER_VIP" || got.UsedBytes == nil || *got.UsedBytes != 0 || got.TotalBytes == nil || *got.TotalBytes != 6597069766656 {
		t.Fatalf("invalid data: %+v", got)
	}
	data, _ := json.Marshal(got)
	if strings.Contains(string(data), "DO_NOT_RETURN") || strings.Contains(string(data), "unconfirmed") {
		t.Fatal("unapproved upstream fields leaked")
	}
}
func TestMembershipDoesNotInventTypeOrCapacity(t *testing.T) {
	for _, tc := range []struct {
		data        string
		wantError   bool
		kind        string
		total, used bool
	}{
		{`{}`, true, "", false, false},
		{`{"member_type":"NORMAL"}`, false, "NORMAL", false, false},
		{`{"member_type":"FUTURE_TIER","total_capacity":10,"use_capacity":11}`, false, "FUTURE_TIER", true, false},
		{`{"member_type":"NORMAL","total_capacity":-1,"use_capacity":"0"}`, false, "NORMAL", false, false},
		{`{"total_capacity":0}`, false, "", true, false},
	} {
		t.Run(tc.data, func(t *testing.T) {
			store, err := newStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			store.setCredential("__puus=example")
			q := newQuark(store)
			q.client.Transport = membershipTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":200,"code":0,"data":` + tc.data + `}`))}, nil
			})
			got, err := q.membership(context.Background())
			if (err != nil) != tc.wantError {
				t.Fatalf("error %v", err)
			}
			if got.Type != tc.kind || (got.TotalBytes != nil) != tc.total || (got.UsedBytes != nil) != tc.used {
				t.Fatalf("invented membership: %+v", got)
			}
		})
	}
}
func TestMembershipFailureDoesNotInvalidateAccountOrLeakResponse(t *testing.T) {
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.setCredential("__puus=example")
	q := newQuark(store)
	q.client.Transport = membershipTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"message":"secret-token"}`))}, nil
	})
	if _, err := q.membership(context.Background()); err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("unsafe failure %v", err)
	}
	cookie, err := store.credential()
	if err != nil || cookie != "__puus=example" {
		t.Fatal("membership failure changed login")
	}
}
