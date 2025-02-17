package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/peterbourgon/ff/v3"
	"github.com/peterbourgon/ff/v3/ffcli"
)

// customUsageFunc is a custom UsageFunc used for all commands.
func customUsageFunc(c *ffcli.Command) string {
	var b strings.Builder

	if c.LongHelp != "" {
		fmt.Fprintf(&b, "%s\n\n", c.LongHelp)
	}

	fmt.Fprintf(&b, "USAGE\n")
	if c.ShortUsage != "" {
		fmt.Fprintf(&b, "  %s\n", c.ShortUsage)
	} else {
		fmt.Fprintf(&b, "  %s\n", c.Name)
	}
	fmt.Fprintf(&b, "\n")

	if len(c.Subcommands) > 0 {
		fmt.Fprintf(&b, "SUBCOMMANDS\n")
		tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
		for _, subcommand := range c.Subcommands {
			fmt.Fprintf(tw, "  %s\t%s\n", subcommand.Name, subcommand.ShortHelp)
		}
		tw.Flush()
		fmt.Fprintf(&b, "\n")
	}

	if countFlags(c.FlagSet) > 0 {
		fmt.Fprintf(&b, "FLAGS\n")
		tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
		type flagUsage struct {
			name         string
			usage        string
			defaultValue string
		}
		flags := []flagUsage{}
		c.FlagSet.VisitAll(func(f *flag.Flag) {
			f1 := flagUsage{name: f.Name, usage: f.Usage, defaultValue: f.DefValue}
			flags = append(flags, f1)
		})

		sort.SliceStable(flags, func(i, j int) bool {
			// sort by the service name between the brackets "[]" found in the usage string.
			r := regexp.MustCompile(`^\[(.*?)\]`)
			return r.FindString(flags[i].usage) < r.FindString(flags[j].usage)
		})
		for _, elem := range flags {
			if elem.defaultValue != "" {
				fmt.Fprintf(tw, "  -%s\t%s (default %q)\n", elem.name, elem.usage, elem.defaultValue)
			} else {
				fmt.Fprintf(tw, "  -%s\t%s\n", elem.name, elem.usage)
			}
		}
		tw.Flush()
		fmt.Fprintf(&b, "\n")
	}

	return strings.TrimSpace(b.String()) + "\n"
}

func countFlags(fs *flag.FlagSet) (n int) {
	fs.VisitAll(func(*flag.Flag) { n++ })

	return n
}

func syslogFlags(c *config, fs *flag.FlagSet) {
	fs.BoolVar(&c.syslog.enabled, "syslog-enabled", true, "[syslog] enable Syslog server(receiver)")
	fs.StringVar(&c.syslog.bindAddr, "syslog-addr", detectPublicIPv4(":514"), "[syslog] local IP:Port to listen on for Syslog messages")
}

func tftpFlags(c *config, fs *flag.FlagSet) {
	fs.BoolVar(&c.tftp.enabled, "tftp-enabled", true, "[tftp] enable iPXE TFTP binary server)")
	fs.StringVar(&c.tftp.bindAddr, "tftp-addr", detectPublicIPv4(":69"), "[tftp] local IP:Port to listen on for iPXE TFTP binary requests")
	fs.DurationVar(&c.tftp.timeout, "tftp-timeout", time.Second*5, "[tftp] iPXE TFTP binary server requests timeout")
	fs.StringVar(&c.tftp.ipxeScriptPatch, "ipxe-script-patch", "", "[tftp/http] iPXE script fragment to patch into served iPXE binaries served via TFTP or HTTP")
	fs.IntVar(&c.tftp.blockSize, "tftp-block-size", 512, "[tftp] TFTP block size a value between 512 (the default block size for TFTP) and 65456 (the max size a UDP packet payload can be)")
}

func ipxeHTTPBinaryFlags(c *config, fs *flag.FlagSet) {
	fs.BoolVar(&c.ipxeHTTPBinary.enabled, "http-ipxe-binary-enabled", true, "[http] enable iPXE HTTP binary server")
}

func ipxeHTTPScriptFlags(c *config, fs *flag.FlagSet) {
	fs.BoolVar(&c.ipxeHTTPScript.enabled, "http-ipxe-script-enabled", true, "[http] enable iPXE HTTP script server")
	fs.StringVar(&c.ipxeHTTPScript.bindAddr, "http-addr", detectPublicIPv4(":80"), "[http] local IP:Port to listen on for iPXE HTTP script requests")
	fs.StringVar(&c.ipxeHTTPScript.extraKernelArgs, "extra-kernel-args", "", "[http] extra set of kernel args (k=v k=v) that are appended to the kernel cmdline iPXE script")
	fs.StringVar(&c.ipxeHTTPScript.trustedProxies, "trusted-proxies", "", "[http] comma separated list of trusted proxies in CIDR notation")
	fs.StringVar(&c.ipxeHTTPScript.hookURL, "osie-url", "", "[http] URL where OSIE (HookOS) images are located")
	fs.StringVar(&c.ipxeHTTPScript.tinkServer, "tink-server", "", "[http] IP:Port for the Tink server")
	fs.BoolVar(&c.ipxeHTTPScript.tinkServerUseTLS, "tink-server-tls", false, "[http] use TLS for Tink server")
}

func dhcpFlags(c *config, fs *flag.FlagSet) {
	fs.BoolVar(&c.dhcp.enabled, "dhcp-enabled", true, "[dhcp] enable DHCP server")
	fs.StringVar(&c.dhcp.bindAddr, "dhcp-addr", "0.0.0.0:67", "[dhcp] local IP:Port to listen on for DHCP requests")
	fs.StringVar(&c.dhcp.bindInterface, "dhcp-iface", "", "[dhcp] interface to bind to for DHCP requests")
	fs.StringVar(&c.dhcp.ipForPacket, "dhcp-ip-for-packet", detectPublicIPv4(""), "[dhcp] IP address to use in DHCP packets (opt 54, etc)")
	fs.StringVar(&c.dhcp.syslogIP, "dhcp-syslog-ip", detectPublicIPv4(""), "[dhcp] Syslog server IP address to use in DHCP packets (opt 7)")
	fs.StringVar(&c.dhcp.tftpIP, "dhcp-tftp-ip", detectPublicIPv4(":69"), "[dhcp] TFTP server IP address to use in DHCP packets (opt 66, etc)")
	fs.StringVar(&c.dhcp.httpIpxeBinaryURL, "dhcp-http-ipxe-binary-url", "http://"+detectPublicIPv4(":8080/ipxe/"), "[dhcp] HTTP iPXE binaries URL to use in DHCP packets")
	fs.StringVar(&c.dhcp.httpIpxeScript.url, "dhcp-http-ipxe-script-url", "http://"+detectPublicIPv4("/auto.ipxe"), "[dhcp] HTTP iPXE script URL to use in DHCP packets")
	fs.BoolVar(&c.dhcp.httpIpxeScript.injectMacAddress, "dhcp-http-ipxe-script-prepend-mac", true, "[dhcp] prepend the hardware MAC address to iPXE script URL base, http://1.2.3.4/auto.ipxe -> http://1.2.3.4/40:15:ff:89:cc:0e/auto.ipxe")
}

func backendFlags(c *config, fs *flag.FlagSet) {
	fs.BoolVar(&c.backends.file.Enabled, "backend-file-enabled", false, "[backend] enable the file backend for DHCP and the HTTP iPXE script")
	fs.StringVar(&c.backends.file.FilePath, "backend-file-path", "", "[backend] the hardware yaml file path for the file backend")
	fs.BoolVar(&c.backends.kubernetes.Enabled, "backend-kube-enabled", true, "[backend] enable the kubernetes backend for DHCP and the HTTP iPXE script")
	fs.StringVar(&c.backends.kubernetes.ConfigFilePath, "backend-kube-config", "", "[backend] the Kubernetes config file location, kube backend only")
	fs.StringVar(&c.backends.kubernetes.APIURL, "backend-kube-api", "", "[backend] the Kubernetes API URL, used for in-cluster client construction, kube backend only")
	fs.StringVar(&c.backends.kubernetes.Namespace, "backend-kube-namespace", "", "[backend] an optional Kubernetes namespace override to query hardware data from, kube backend only")
}

func setFlags(c *config, fs *flag.FlagSet) {
	fs.StringVar(&c.logLevel, "log-level", "info", "log level (debug, info)")
	dhcpFlags(c, fs)
	tftpFlags(c, fs)
	ipxeHTTPBinaryFlags(c, fs)
	ipxeHTTPScriptFlags(c, fs)
	syslogFlags(c, fs)
	backendFlags(c, fs)
}

func newCLI(cfg *config, fs *flag.FlagSet) *ffcli.Command {
	setFlags(cfg, fs)
	return &ffcli.Command{
		Name:       name,
		ShortUsage: "smee [flags]",
		LongHelp:   "Smee is the DHCP and Network boot service for use in the Tinkerbell stack.",
		FlagSet:    fs,
		Options:    []ff.Option{ff.WithEnvVarPrefix(name)},
		UsageFunc:  customUsageFunc,
	}
}

func detectPublicIPv4(extra string) string {
	ip, err := autoDetectPublicIPv4()
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%v%v", ip.String(), extra)
}

func autoDetectPublicIPv4() (net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, fmt.Errorf("unable to auto-detect public IPv4: %w", err)
	}
	for _, addr := range addrs {
		ip, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		v4 := ip.IP.To4()
		if v4 == nil || !v4.IsGlobalUnicast() {
			continue
		}

		return v4, nil
	}

	return nil, errors.New("unable to auto-detect public IPv4")
}
