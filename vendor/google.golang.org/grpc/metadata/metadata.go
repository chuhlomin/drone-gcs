/*
 *
 * Copyright 2014, Google Inc.
 * All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions are
 * met:
 *
 *     * Redistributions of source code must retain the above copyright
 * notice, this list of conditions and the following disclaimer.
 *     * Redistributions in binary form must reproduce the above
 * copyright notice, this list of conditions and the following disclaimer
 * in the documentation and/or other materials provided with the
 * distribution.
 *     * Neither the name of Google Inc. nor the names of its
 * contributors may be used to endorse or promote products derived from
 * this software without specific prior written permission.
 *
 * THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
 * "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
 * LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
 * A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
 * OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
 * SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
 * LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
 * DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
 * THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 * (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
 * OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 *
 */

// Package metadata define the structure of the metadata supported by gRPC library.
// Please refer to http://www.grpc.io/docs/guides/wire.html for more information about custom-metadata.
package metadata // import "google.golang.org/grpc/metadata"

import (
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/net/context"
)

const (
	binHdrSuffix = "-bin"
)

// encodeKeyValue encodes key and value qualified for transmission via gRPC.
// Transmitting binary headers violates HTTP/2 spec.
// TODO(zhaoq): Maybe check if k is ASCII also.
func encodeKeyValue(k, v string) (string, string) {
	k = strings.ToLower(k)
	if strings.HasSuffix(k, binHdrSuffix) {
		val := base64.StdEncoding.EncodeToString([]byte(v))
		v = string(val)
	}
	return k, v
}

// DecodeKeyValue returns the original key and value corresponding to the
// encoded data in k, v.
// If k is a binary header and v contains comma, v is split on comma before decoded,
// and the decoded v will be joined with comma before returned.
func DecodeKeyValue(k, v string) (string, string, error) {
	if !strings.HasSuffix(k, binHdrSuffix) {
		return k, v, nil
	}
	vvs := strings.Split(v, ",")
	for i, vv := range vvs {
		val, err := base64.StdEncoding.DecodeString(vv)
		if err != nil {
			return "", "", err
		}
		vvs[i] = string(val)
	}
	return k, strings.Join(vvs, ","), nil
}

// MD is a mapping from metadata keys to values. Users should use the following
// two convenience functions New and Pairs to generate MD.
type MD map[string][]string

// New creates a MD from given key-value map.
// Keys are automatically converted to lowercase. And for keys having "-bin" as suffix, their values will be applied Base64 encoding.
func New(m map[string]string) MD {
	md := MD{}
	for k, v := range m {
		key, val := encodeKeyValue(k, v)
		md[key] = append(md[key], val)
	}
	return md
}

// Pairs returns an MD formed by the mapping of key, value ...
// Pairs panics if len(kv) is odd.
// Keys are automatically converted to lowercase. And for keys having "-bin" as suffix, their values will be appplied Base64 encoding.
func Pairs(kv ...string) MD {
	if len(kv)%2 == 1 {
		panic(fmt.Sprintf("metadata: Pairs got the odd number of input pairs for metadata: %d", len(kv)))
	}
	md := MD{}
	var k string
	for i, s := range kv {
		if i%2 == 0 {
			k = s
			continue
		}
		key, val := encodeKeyValue(k, s)
		md[key] = append(md[key], val)
	}
	return md
}

// Len returns the number of items in md.
func (md MD) Len() int {
	return len(md)
}

// Copy returns a copy of md.
func (md MD) Copy() MD {
	return Join(md)
}

// Join joins any number of MDs into a single MD.
// The order of values for each key is determined by the order in which
// the MDs containing those values are presented to Join.
func Join(mds ...MD) MD {
	out := MD{}
	for _, md := range mds {
		for k, v := range md {
			out[k] = append(out[k], v...)
		}
	}
	return out
}

type mdIncomingKey struct{}
type mdOutgoingKey struct{}

// NewContext is a wrapper for NewOutgoingContext(ctx, md).  Deprecated.
func NewContext(ctx context.Context, md MD) context.Context {
	return NewOutgoingContext(ctx, md)
}

// NewIncomingContext creates a new context with incoming md attached.
func NewIncomingContext(ctx context.Context, md MD) context.Context {
	return context.WithValue(ctx, mdIncomingKey{}, md)
}

// NewOutgoingContext creates a new context with outgoing md attached.
func NewOutgoingContext(ctx context.Context, md MD) context.Context {
	return context.WithValue(ctx, mdOutgoingKey{}, md)
}

// FromContext is a wrapper for FromIncomingContext(ctx).  Deprecated.
func FromContext(ctx context.Context) (md MD, ok bool) {
	return FromIncomingContext(ctx)
}

// FromIncomingContext returns the incoming MD in ctx if it exists.  The
// returned md should be immutable, writing to it may cause races.
// Modification should be made to the copies of the returned md.
func FromIncomingContext(ctx context.Context) (md MD, ok bool) {
	md, ok = ctx.Value(mdIncomingKey{}).(MD)
	return
}

// FromOutgoingContext returns the outgoing MD in ctx if it exists.  The
// returned md should be immutable, writing to it may cause races.
// Modification should be made to the copies of the returned md.
func FromOutgoingContext(ctx context.Context) (md MD, ok bool) {
	md, ok = ctx.Value(mdOutgoingKey{}).(MD)
	return
}
