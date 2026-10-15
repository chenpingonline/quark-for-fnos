package main

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"
)

type Membership struct {
	Type       string `json:"type,omitempty"`
	UsedBytes  *int64 `json:"used_bytes,omitempty"`
	TotalBytes *int64 `json:"total_bytes,omitempty"`
}

// /member fields are also consumed by OpenList's Quark driver. Return only
// display fields, never the raw upstream response or account tokens.
func (q *Quark) membership(ctx context.Context) (Membership, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	m, err := q.request(ctx, "GET", driveBase+"/member", nil, nil, true)
	if err != nil {
		return Membership{}, errors.New("会员信息暂不可用")
	}
	d := dataMap(m)
	info := Membership{}
	if kind, ok := d["member_type"].(string); ok && len(kind) <= 64 {
		info.Type = strings.TrimSpace(kind)
	}
	info.UsedBytes = memberCapacity(d["use_capacity"])
	info.TotalBytes = memberCapacity(d["total_capacity"])
	if info.TotalBytes != nil && info.UsedBytes != nil && *info.UsedBytes > *info.TotalBytes {
		info.UsedBytes = nil
	}
	if info.Type == "" && info.TotalBytes == nil && info.UsedBytes == nil {
		return Membership{}, errors.New("会员信息暂不可用")
	}
	return info, nil
}
func memberCapacity(v any) *int64 {
	n, ok := v.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n >= float64(math.MaxInt64) || math.Trunc(n) != n {
		return nil
	}
	value := int64(n)
	return &value
}
