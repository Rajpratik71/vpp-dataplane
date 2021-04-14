// Copyright (C) 2019 Cisco Systems Inc.
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

package startup

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/projectcalico/vpp-dataplane/vpp-manager/config"
	"github.com/projectcalico/vpp-dataplane/vpp-manager/utils"
	"github.com/projectcalico/vpp-dataplane/vpplink/types"
	log "github.com/sirupsen/logrus"
)

const (
	NodeNameEnvVar             = "NODENAME"
	IpConfigEnvVar             = "CALICOVPP_IP_CONFIG"
	RxModeEnvVar               = "CALICOVPP_RX_MODE"
	NumRxQueuesEnvVar          = "CALICOVPP_RX_QUEUES"
	TapRxModeEnvVar            = "CALICOVPP_TAP_RX_MODE"
	InterfaceEnvVar            = "CALICOVPP_INTERFACE"
	ConfigTemplateEnvVar       = "CALICOVPP_CONFIG_TEMPLATE"
	ConfigExecTemplateEnvVar   = "CALICOVPP_CONFIG_EXEC_TEMPLATE"
	InitScriptTemplateEnvVar   = "CALICOVPP_INIT_SCRIPT_TEMPLATE"
	IfConfigPathEnvVar         = "CALICOVPP_IF_CONFIG_PATH"
	VppStartupSleepEnvVar      = "CALICOVPP_VPP_STARTUP_SLEEP"
	ExtraAddrCountEnvVar       = "CALICOVPP_CONFIGURE_EXTRA_ADDRESSES"
	CorePatternEnvVar          = "CALICOVPP_CORE_PATTERN"
	TapRingSizeEnvVar          = "CALICOVPP_TAP_RING_SIZE"
	UserSpecifiedMtuEnvVar     = "CALICOVPP_TAP_MTU"
	IpsecNbAsyncCryptoThEnvVar = "CALICOVPP_IPSEC_NB_ASYNC_CRYPTO_THREAD"
	RingSizeEnvVar             = "CALICOVPP_RING_SIZE"
	NativeDriverEnvVar         = "CALICOVPP_NATIVE_DRIVER"
	SwapDriverEnvVar           = "CALICOVPP_SWAP_DRIVER"
	DefaultGWEnvVar            = "CALICOVPP_DEFAULT_GW"
	EnableGSOEnvVar            = "CALICOVPP_DEBUG_ENABLE_GSO"
	ServicePrefixEnvVar        = "SERVICE_PREFIX"
)

const (
	DefaultTapQueueSize = 1024
	DefaultPhyQueueSize = 1024
	DefaultNumRxQueues  = 1
	defaultRxMode       = types.Adaptative
)

func getVppManagerParams() (params *config.VppManagerParams) {
	params = &config.VppManagerParams{}
	err := parseEnvVariables(params)
	if err != nil {
		log.Panicf("Parse error %v", err)
	}
	getSystemCapabilities(params)
	return params
}

func getSystemCapabilities(params *config.VppManagerParams) {
	/* Drivers */
	params.LoadedDrivers = make(map[string]bool)
	vfioLoaded, err := utils.IsDriverLoaded(config.DRIVER_VFIO_PCI)
	if err != nil {
		log.Warnf("Error determining whether %s is loaded", config.DRIVER_VFIO_PCI)
	}
	params.LoadedDrivers[config.DRIVER_VFIO_PCI] = vfioLoaded
	uioLoaded, err := utils.IsDriverLoaded(config.DRIVER_UIO_PCI_GENERIC)
	if err != nil {
		log.Warnf("Error determining whether %s is loaded", config.DRIVER_UIO_PCI_GENERIC)
	}
	params.LoadedDrivers[config.DRIVER_UIO_PCI_GENERIC] = uioLoaded

	/* AF XDP support */
	kernel, err := utils.GetOsKernelVersion()
	if err != nil {
		log.Warnf("Error getting os kernel version %v", err)
	} else {
		params.KernelVersion = kernel
	}

	/* Hugepages */
	nrHugepages, err := utils.GetNrHugepages()
	if err != nil {
		log.Warnf("Error getting nrHugepages %v", err)
	}
	params.AvailableHugePages = nrHugepages

	/* Iommu */
	iommu, err := utils.IsVfioUnsafeiommu()
	if err != nil {
		log.Warnf("Error getting vfio iommu state %v", err)
	}
	params.VfioUnsafeiommu = iommu

}

var supportedEnvVars map[string]bool

func isEnvVarSupported(str string) bool {
	_, found := supportedEnvVars[str]
	return found
}

func getEnvValue(str string) string {
	supportedEnvVars[str] = true
	return os.Getenv(str)
}

func parseEnvVariables(params *config.VppManagerParams) (err error) {
	supportedEnvVars = make(map[string]bool)

	vppStartupSleep := getEnvValue(VppStartupSleepEnvVar)
	if vppStartupSleep == "" {
		params.VppStartupSleepSeconds = 0
	} else {
		i, err := strconv.ParseInt(vppStartupSleep, 10, 32)
		params.VppStartupSleepSeconds = int(i)
		if err != nil {
			return errors.Wrapf(err, "Error Parsing %s", VppStartupSleepEnvVar)
		}
	}

	params.MainInterface = getEnvValue(InterfaceEnvVar)
	if params.MainInterface == "" {
		return errors.Errorf("No interface specified. Specify an interface through the %s environment variable", InterfaceEnvVar)
	}

	params.ConfigExecTemplate = getEnvValue(ConfigExecTemplateEnvVar)
	params.InitScriptTemplate = getEnvValue(InitScriptTemplateEnvVar)

	params.ConfigTemplate = getEnvValue(ConfigTemplateEnvVar)
	if params.ConfigTemplate == "" {
		return fmt.Errorf("empty VPP configuration template, set a template in the %s environment variable", ConfigTemplateEnvVar)
	}

	params.IfConfigSavePath = getEnvValue(IfConfigPathEnvVar)

	params.NodeName = getEnvValue(NodeNameEnvVar)
	if params.NodeName == "" {
		return errors.Errorf("No node name specified. Specify the NODENAME environment variable")
	}

	servicePrefixStr := getEnvValue(ServicePrefixEnvVar)
	for _, prefixStr := range strings.Split(servicePrefixStr, ",") {
		_, serviceCIDR, err := net.ParseCIDR(prefixStr)
		if err != nil {
			return errors.Errorf("invalid service prefix configuration: %s %s", prefixStr, err)
		}
		params.ServiceCIDRs = append(params.ServiceCIDRs, *serviceCIDR)
	}

	params.VppIpConfSource = getEnvValue(IpConfigEnvVar)
	if params.VppIpConfSource != "linux" { // TODO add dhcp, config file, etc.
		return errors.Errorf("No ip configuration source specified. Specify one of {linux,} through the %s environment variable", IpConfigEnvVar)
	}

	params.CorePattern = getEnvValue(CorePatternEnvVar)

	params.ExtraAddrCount = 0
	if extraAddrConf := getEnvValue(ExtraAddrCountEnvVar); extraAddrConf != "" {
		extraAddrCount, err := strconv.ParseInt(extraAddrConf, 10, 8)
		if err != nil {
			log.Errorf("Couldn't parse %s: %v", ExtraAddrCountEnvVar, err)
		} else {
			params.ExtraAddrCount = int(extraAddrCount)
		}
	}

	params.NativeDriver = ""
	if conf := getEnvValue(NativeDriverEnvVar); conf != "" {
		params.NativeDriver = strings.ToLower(conf)
	}

	params.NumRxQueues = DefaultNumRxQueues
	if conf := getEnvValue(NumRxQueuesEnvVar); conf != "" {
		queues, err := strconv.ParseInt(conf, 10, 16)
		if err != nil || queues <= 0 {
			log.Errorf("Invalid %s configuration: %s parses to %d err %v", NumRxQueuesEnvVar, conf, queues, err)
		} else {
			params.NumRxQueues = int(queues)
		}
	}

	params.NewDriverName = getEnvValue(SwapDriverEnvVar)

	params.RxMode = types.UnformatRxMode(getEnvValue(RxModeEnvVar))
	if params.RxMode == types.UnknownRxMode {
		params.RxMode = defaultRxMode
	}
	params.TapRxMode = types.UnformatRxMode(getEnvValue(TapRxModeEnvVar))
	if params.TapRxMode == types.UnknownRxMode {
		params.TapRxMode = defaultRxMode
	}

	if conf := getEnvValue(DefaultGWEnvVar); conf != "" {
		for _, defaultGWStr := range strings.Split(conf, ",") {
			defaultGW := net.ParseIP(defaultGWStr)
			if defaultGW == nil {
				return errors.Errorf("Unable to parse IP: %s", conf)
			}
			params.DefaultGWs = append(params.DefaultGWs, defaultGW)
		}
	}

	if conf := getEnvValue(UserSpecifiedMtuEnvVar); conf != "" {
		userSpecifiedMtu, err := strconv.ParseInt(conf, 10, 32)
		if err != nil {
			return fmt.Errorf("Invalid %s configuration: %s parses to %v err %v", UserSpecifiedMtuEnvVar, conf, userSpecifiedMtu, err)
		}
		params.UserSpecifiedMtu = int(userSpecifiedMtu)
	} else {
		params.UserSpecifiedMtu = 0
	}

	params.TapRxQueueSize = DefaultTapQueueSize
	params.TapTxQueueSize = DefaultTapQueueSize
	if conf := getEnvValue(TapRingSizeEnvVar); conf != "" {
		params.TapRxQueueSize, params.TapTxQueueSize, err = parseRingSize(conf)
		if err != nil {
			return errors.Wrapf(err, "Error parsing %s", TapRingSizeEnvVar)
		}
	}

	params.RxQueueSize = DefaultPhyQueueSize
	params.TxQueueSize = DefaultPhyQueueSize
	if conf := getEnvValue(RingSizeEnvVar); conf != "" {
		params.RxQueueSize, params.TxQueueSize, err = parseRingSize(conf)
		if err != nil {
			return errors.Wrapf(err, "Error parsing %s", RingSizeEnvVar)
		}
	}

	params.EnableGSO = true
	if conf := getEnvValue(EnableGSOEnvVar); conf != "" {
		enableGSO, err := strconv.ParseBool(conf)
		if err != nil {
			return fmt.Errorf("Invalid %s configuration: %s parses to %v err %v", EnableGSOEnvVar, conf, enableGSO, err)
		}
		params.EnableGSO = enableGSO
	}

	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		if strings.Contains(pair[0], "CALICOVPP_") {
			if !isEnvVarSupported(pair[0]) {
				log.Warnf("Environment variable %s is not supported", pair[0])
			}
		}
	}
	return nil
}

func parseRingSize(conf string) (int, int, error) {
	rxSize := 0
	txSize := 0
	if conf == "" {
		return 0, 0, fmt.Errorf("Empty configuration")
	}
	sizes := strings.Split(conf, ",")
	if len(sizes) == 1 {
		sz, err := strconv.ParseInt(sizes[0], 10, 32)
		if err != nil {
			return 0, 0, fmt.Errorf("Invalid conf: %s parses to %v err %v", conf, sz, err)
		}
		rxSize = int(sz)
		txSize = int(sz)
	} else if len(sizes) == 2 {
		sz, err := strconv.ParseInt(sizes[0], 10, 32)
		if err != nil {
			return 0, 0, fmt.Errorf("Invalid conf: %s parses to %v err %v", conf, sz, err)
		}
		rxSize = int(sz)
		sz, err = strconv.ParseInt(sizes[1], 10, 32)
		if err != nil {
			return 0, 0, fmt.Errorf("Invalid conf: %s parses to %v err %v", conf, sz, err)
		}
		txSize = int(sz)
	} else {
		return 0, 0, fmt.Errorf("Invalid conf: %s parses to %v", conf, sizes)
	}
	return rxSize, txSize, nil
}

func PrintVppManagerConfig(params *config.VppManagerParams, conf *config.InterfaceConfig) {
	log.Infof("-- Environment --")
	log.Infof("CorePattern:         %s", params.CorePattern)
	log.Infof("ExtraAddrCount:      %d", params.ExtraAddrCount)
	log.Infof("Native driver:       %s", params.NativeDriver)
	log.Infof("RxMode:              %s", types.FormatRxMode(params.RxMode))
	log.Infof("TapRxMode:           %s", types.FormatRxMode(params.TapRxMode))
	log.Infof("Service CIDRs:       [%s]", utils.FormatIPNetSlice(params.ServiceCIDRs))
	log.Infof("Tap Queue Size:      rx:%d tx:%d", params.TapRxQueueSize, params.TapTxQueueSize)
	log.Infof("PHY Queue Size:      rx:%d tx:%d", params.RxQueueSize, params.TxQueueSize)
	log.Infof("PHY target #Queues   rx:%d", params.NumRxQueues)
	log.Infof("Hugepages            %d", params.AvailableHugePages)
	log.Infof("KernelVersion        %s", params.KernelVersion)
	log.Infof("Drivers              %s", params.LoadedDrivers)
	log.Infof("vfio iommu:          %t", params.VfioUnsafeiommu)

	log.Infof("-- Interface config --")
	log.Infof("Node IP4:            %s", conf.NodeIP4)
	log.Infof("Node IP6:            %s", conf.NodeIP6)
	log.Infof("PciId:               %s", conf.PciId)
	log.Infof("Driver:              %s", conf.Driver)
	log.Infof("Linux IF was up ?    %t", conf.IsUp)
	log.Infof("Promisc was on ?     %t", conf.PromiscOn)
	log.Infof("DoSwapDriver:        %t", conf.DoSwapDriver)
	log.Infof("Mac:                 %s", conf.HardwareAddr.String())
	log.Infof("Addresses:           [%s]", conf.AddressString())
	log.Infof("Routes:              [%s]", conf.RouteString())
	log.Infof("PHY original #Queues rx:%d tx:%d", conf.NumRxQueues, conf.NumTxQueues)
	log.Infof("MTU                  %d", conf.Mtu)
}

func runInitScript(params *config.VppManagerParams) error {
	if params.InitScriptTemplate == "" {
		return nil
	}
	// Trivial rendering for the moment...
	template := strings.ReplaceAll(params.InitScriptTemplate, "__VPP_DATAPLANE_IF__", params.MainInterface)
	cmd := exec.Command("/bin/bash", "-c", template)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func PrepareConfiguration() (params *config.VppManagerParams, conf *config.InterfaceConfig) {
	params = getVppManagerParams()
	err := utils.ClearVppManagerFiles()
	if err != nil {
		log.Fatalf("Error clearing config files: %+v", err)
	}

	err = utils.SetCorePattern(params.CorePattern)
	if err != nil {
		log.Fatalf("Error setting core pattern: %s", err)
	}

	err = utils.SetRLimitMemLock()
	if err != nil {
		log.Errorf("Error raising memlock limit, VPP may fail to start: %v", err)
	}

	/* Run this before getLinuxConfig() in case this is a script
	 * that's responsible for creating the interface */
	err = runInitScript(params)
	if err != nil {
		log.Fatalf("Error running init script: %s", err)
	}

	conf, err = getInterfaceConfig(params)
	if err != nil {
		log.Fatalf("Error getting initial interface configuration: %s", err)
	}

	return params, conf
}
