// Code generated by GoVPP's binapi-generator. DO NOT EDIT.
// versions:
//  binapi-generator: v0.4.0-dev
//  VPP:              unknown

// Package nat_types contains generated bindings for API file nat_types.api.
//
// Contents:
//   1 enum
//
package nat_types

import (
	"strconv"

	api "git.fd.io/govpp.git/api"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the GoVPP api package it is being compiled against.
// A compilation error at this line likely means your copy of the
// GoVPP api package needs to be updated.
const _ = api.GoVppAPIPackageIsVersion2

// NatConfigFlags defines enum 'nat_config_flags'.
type NatConfigFlags uint8

const (
	NAT_IS_NONE           NatConfigFlags = 0
	NAT_IS_TWICE_NAT      NatConfigFlags = 1
	NAT_IS_SELF_TWICE_NAT NatConfigFlags = 2
	NAT_IS_OUT2IN_ONLY    NatConfigFlags = 4
	NAT_IS_ADDR_ONLY      NatConfigFlags = 8
	NAT_IS_OUTSIDE        NatConfigFlags = 16
	NAT_IS_INSIDE         NatConfigFlags = 32
	NAT_IS_STATIC         NatConfigFlags = 64
	NAT_IS_EXT_HOST_VALID NatConfigFlags = 128
)

var (
	NatConfigFlags_name = map[uint8]string{
		0:   "NAT_IS_NONE",
		1:   "NAT_IS_TWICE_NAT",
		2:   "NAT_IS_SELF_TWICE_NAT",
		4:   "NAT_IS_OUT2IN_ONLY",
		8:   "NAT_IS_ADDR_ONLY",
		16:  "NAT_IS_OUTSIDE",
		32:  "NAT_IS_INSIDE",
		64:  "NAT_IS_STATIC",
		128: "NAT_IS_EXT_HOST_VALID",
	}
	NatConfigFlags_value = map[string]uint8{
		"NAT_IS_NONE":           0,
		"NAT_IS_TWICE_NAT":      1,
		"NAT_IS_SELF_TWICE_NAT": 2,
		"NAT_IS_OUT2IN_ONLY":    4,
		"NAT_IS_ADDR_ONLY":      8,
		"NAT_IS_OUTSIDE":        16,
		"NAT_IS_INSIDE":         32,
		"NAT_IS_STATIC":         64,
		"NAT_IS_EXT_HOST_VALID": 128,
	}
)

func (x NatConfigFlags) String() string {
	s, ok := NatConfigFlags_name[uint8(x)]
	if ok {
		return s
	}
	str := func(n uint8) string {
		s, ok := NatConfigFlags_name[uint8(n)]
		if ok {
			return s
		}
		return "NatConfigFlags(" + strconv.Itoa(int(n)) + ")"
	}
	for i := uint8(0); i <= 8; i++ {
		val := uint8(x)
		if val&(1<<i) != 0 {
			if s != "" {
				s += "|"
			}
			s += str(1 << i)
		}
	}
	if s == "" {
		return str(uint8(x))
	}
	return s
}
