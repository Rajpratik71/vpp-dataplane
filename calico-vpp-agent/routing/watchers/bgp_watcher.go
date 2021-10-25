// Copyright (C) 2020 Cisco Systems Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package watchers

import (
	"fmt"
	"net"

	"github.com/golang/protobuf/ptypes"
	bgpapi "github.com/osrg/gobgp/api"
	"github.com/pkg/errors"
	commonAgent "github.com/projectcalico/vpp-dataplane/calico-vpp-agent/common"
	"github.com/projectcalico/vpp-dataplane/calico-vpp-agent/config"
	"github.com/projectcalico/vpp-dataplane/calico-vpp-agent/routing/common"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/protobuf/types/known/anypb"
)

type BGPWatcher struct {
	*common.RoutingData
	*commonAgent.CalicoVppServerData
	log      *logrus.Entry
	reloadCh chan string
}

func (w *BGPWatcher) getNexthop(path *bgpapi.Path) string {
	for _, attr := range path.Pattrs {
		nhAttr := &bgpapi.NextHopAttribute{}
		mpReachAttr := &bgpapi.MpReachNLRIAttribute{}
		if err := ptypes.UnmarshalAny(attr, nhAttr); err == nil {
			return nhAttr.NextHop
		}
		if err := ptypes.UnmarshalAny(attr, mpReachAttr); err == nil {
			if len(mpReachAttr.NextHops) != 1 {
				w.log.Fatalf("Cannot process more than one Nlri in path attributes: %+v", mpReachAttr)
			}
			return mpReachAttr.NextHops[0]
		}
	}
	return ""
}

// injectRoute is a helper function to inject BGP routes to VPP
// TODO: multipath support
func (w *BGPWatcher) injectRoute(path *bgpapi.Path) error {
	var dst net.IPNet
	ipAddrPrefixNlri := &bgpapi.IPAddressPrefix{}
	otherNodeIP := net.ParseIP(w.getNexthop(path))
	if otherNodeIP == nil {
		return fmt.Errorf("Cannot determine path nexthop: %+v", path)
	}

	if err := ptypes.UnmarshalAny(path.Nlri, ipAddrPrefixNlri); err == nil {
		dst.IP = net.ParseIP(ipAddrPrefixNlri.Prefix)
		if dst.IP == nil {
			return fmt.Errorf("Cannot parse nlri addr: %s", ipAddrPrefixNlri.Prefix)
		} else if dst.IP.To4() == nil {
			dst.Mask = net.CIDRMask(int(ipAddrPrefixNlri.PrefixLen), 128)
		} else {
			dst.Mask = net.CIDRMask(int(ipAddrPrefixNlri.PrefixLen), 32)
		}
	} else {
		return fmt.Errorf("Cannot handle Nlri: %+v", path.Nlri)
	}

	cn := &common.NodeConnectivity{
		Dst:     dst,
		NextHop: otherNodeIP,
	}
	if path.IsWithdraw {
		w.ConnectivityEventChan <- common.ConnectivityEvent{
			Type: common.ConnectivtyDeleted,
			Old:  cn,
		}
	} else {
		w.ConnectivityEventChan <- common.ConnectivityEvent{
			Type: common.ConnectivtyAdded,
			New:  cn,
		}
	}
	return nil
}

func (w *BGPWatcher) getSRv6SegList(path *bgpapi.Path) (srv6tunnel *common.SRv6Tunnel, srnrli *bgpapi.SRPolicyNLRI, err error) {
	w.log.Infof("getSRv6SegList ")
	srv6tunnel = &common.SRv6Tunnel{}
	srnrli = &bgpapi.SRPolicyNLRI{}
	if err := ptypes.UnmarshalAny(path.Nlri, srnrli); err == nil {
		w.log.Infof("Got path update with SRv6BindingSID")
	}
	srv6tunnel.Dst = net.IP(srnrli.Endpoint)

	for _, attr := range path.Pattrs {
		w.log.Infof("getSegList1 %s", attr)
		tun := &bgpapi.TunnelEncapAttribute{}
		if err := ptypes.UnmarshalAny(attr, tun); err == nil {
			for _, tlv := range tun.Tlvs {
				for _, innerTlv := range tlv.Tlvs {
					segList := &bgpapi.TunnelEncapSubTLVSRSegmentList{}
					if err := ptypes.UnmarshalAny(innerTlv, segList); err == nil {
						w.log.Infof("getSegList2 %s", segList.Segments)

						for _, seglist := range segList.Segments {
							segment := &bgpapi.SegmentTypeB{}
							if err = ptypes.UnmarshalAny(seglist, segment); err == nil {

								srv6tunnel.Sid = net.IP(segment.GetSid())
								srv6tunnel.Behavior = uint8(segment.GetEndpointBehaviorStructure().Behavior)
								break
							}
						}

					}

					srbsids := &anypb.Any{}
					w.log.Infof("getSegList23 srbsid %s", innerTlv.String())
					if err := ptypes.UnmarshalAny(innerTlv, srbsids); err == nil {
						w.log.Infof("getSegList3 srbsid %s", srbsids.String())
						srv6bsid := &bgpapi.SRBindingSID{}
						if err := ptypes.UnmarshalAny(srbsids, srv6bsid); err == nil {
							srv6tunnel.Bsid = srv6bsid.Sid
							w.log.Infof("getSegList4 %s", net.IP(srv6bsid.Sid).String())
						}

					}

				}
			}
		}

	}
	return srv6tunnel, srnrli, err
}

func (w *BGPWatcher) injectSRv6Policy(path *bgpapi.Path) error {
	w.log.Infof("injectSRv6Policy")

	srtunnel, srnrli, err := w.getSRv6SegList(path)
	if err != nil {
		return errors.Wrap(err, "error injectSRv6Policy")
	}

	cn := &common.NodeConnectivity{
		Dst:              net.IPNet{},
		NextHop:          srnrli.Endpoint,
		ResolvedProvider: "",
		Custom:           srtunnel,
	}
	if path.IsWithdraw {
		w.ConnectivityEventChan <- common.ConnectivityEvent{
			Type: common.SRv6PolicyDeleted,
			Old:  cn,
		}
	} else {
		w.ConnectivityEventChan <- common.ConnectivityEvent{
			Type: common.SRv6PolicyAdded,
			New:  cn,
		}
	}
	return nil
}

// watchBGPPath watches BGP routes from other peers and inject them into
// linux kernel
// TODO: multipath support
func (w *BGPWatcher) WatchBGPPath() error {
	var err error
	startMonitor := func(f *bgpapi.Family) (context.CancelFunc, error) {
		ctx, stopFunc := context.WithCancel(context.Background())
		err := w.BGPServer.MonitorTable(
			ctx,
			&bgpapi.MonitorTableRequest{
				TableType: bgpapi.TableType_GLOBAL,
				Name:      "",
				Family:    f,
				Current:   false,
			},
			func(path *bgpapi.Path) {
				if path == nil {
					w.log.Warnf("nil path update, skipping")
					return
				}
				w.log.Printf("Path monitor family %s, path %s", f.String(), path.String())
				if f == &common.BgpFamilySRv6IPv6 {
					w.log.Debugf("Path SRv6")
					w.BarrierSync()
					if err := w.injectSRv6Policy(path); err != nil {
						w.log.Errorf("cannot inject SRv6: %v", err)
					}
					return
				}
				w.log.Infof("Got path update from %s as %d", path.SourceId, path.SourceAsn)
				if path.NeighborIp == "<nil>" { // Weird GoBGP API behaviour
					w.log.Debugf("Ignoring internal path")
					return
				}
				w.BarrierSync()
				if err := w.injectRoute(path); err != nil {
					w.log.Errorf("cannot inject route: %v", err)
				}
			},
		)
		return stopFunc, err
	}

	var stopV4Monitor, stopV6Monitor, stopSRv6IP6Monitor context.CancelFunc
	if w.HasV4 {
		stopV4Monitor, err = startMonitor(&common.BgpFamilyUnicastIPv4)
		if err != nil {
			return errors.Wrap(err, "error starting v4 path monitor")
		}

	}
	if w.HasV6 {
		stopV6Monitor, err = startMonitor(&common.BgpFamilyUnicastIPv6)
		if err != nil {
			return errors.Wrap(err, "error starting SRv6IP6 path monitor")
		}
		if config.EnableSRv6 {
			stopSRv6IP6Monitor, err = startMonitor(&common.BgpFamilySRv6IPv6)
			if err != nil {
				return errors.Wrap(err, "error starting SRv6IP6 path monitor")
			}
		}

	}
	for family := range w.reloadCh {
		if w.HasV4 && family == "4" {
			stopV4Monitor()
			stopV4Monitor, err = startMonitor(&common.BgpFamilyUnicastIPv4)
			if err != nil {
				return err
			}

		} else if w.HasV6 && family == "6" {
			stopV6Monitor()
			stopV6Monitor, err = startMonitor(&common.BgpFamilyUnicastIPv6)
			if err != nil {
				return err
			}
			if config.EnableSRv6 {
				stopSRv6IP6Monitor()
				stopSRv6IP6Monitor, err = startMonitor(&common.BgpFamilySRv6IPv6)
				if err != nil {
					return errors.Wrap(err, "error starting SRv6IP6 path monitor")
				}
			}
		}
	}
	return nil
}

func NewBGPWatcher(routingData *common.RoutingData, log *logrus.Entry) *BGPWatcher {
	w := BGPWatcher{
		RoutingData: routingData,
		log:         log,
		reloadCh:    make(chan string),
	}
	return &w
}
