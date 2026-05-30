package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newFrameCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "frame",
		Short: "Send and inspect Ethernet frames over UDP for usermode testing",
	}

	cmd.AddCommand(newFrameSendCmd())
	cmd.AddCommand(newFrameListenCmd())

	return cmd
}

func newFrameSendCmd() *cobra.Command {
	var (
		to      string
		srcMAC  string
		dstMAC  string
		payload string
		etherTy uint16
	)

	cmd := &cobra.Command{
		Use:   "send",
		Short: "Craft and send one Ethernet frame over UDP",
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := net.ParseMAC(srcMAC)
			if err != nil {
				return fmt.Errorf("parse src mac: %w", err)
			}
			dst, err := net.ParseMAC(dstMAC)
			if err != nil {
				return fmt.Errorf("parse dst mac: %w", err)
			}

			frame := make([]byte, 0, 14+len(payload))
			frame = append(frame, dst...)
			frame = append(frame, src...)
			frame = append(frame, byte(etherTy>>8), byte(etherTy)) // #nosec G115
			frame = append(frame, []byte(payload)...)

			var dialer net.Dialer
			conn, err := dialer.DialContext(cmd.Context(), "udp", to)
			if err != nil {
				return fmt.Errorf("dial udp %s: %w", to, err)
			}
			defer conn.Close()

			if _, err := conn.Write(frame); err != nil {
				return fmt.Errorf("write frame: %w", err)
			}

			fmt.Printf("sent %d-byte frame to %s\n", len(frame), to)
			return nil
		},
	}

	cmd.Flags().StringVar(&to, "to", "127.0.0.1:10001", "UDP address to send the frame to")
	cmd.Flags().StringVar(&srcMAC, "src", "02:00:00:00:00:01", "source MAC address")
	cmd.Flags().StringVar(&dstMAC, "dst", "ff:ff:ff:ff:ff:ff", "destination MAC address")
	cmd.Flags().StringVar(&payload, "payload", "hello", "ASCII payload to append after the Ethernet header")
	cmd.Flags().Uint16Var(&etherTy, "ethertype", 0x0800, "EtherType field")

	return cmd
}

func newFrameListenCmd() *cobra.Command {
	var (
		listen string
		count  int
		wait   time.Duration
	)

	cmd := &cobra.Command{
		Use:   "listen",
		Short: "Listen on UDP and print received frames",
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, err := net.ResolveUDPAddr("udp", listen)
			if err != nil {
				return fmt.Errorf("resolve udp addr: %w", err)
			}

			conn, err := net.ListenUDP("udp", addr)
			if err != nil {
				return fmt.Errorf("listen udp %s: %w", listen, err)
			}
			defer conn.Close()

			if wait > 0 {
				if err := conn.SetReadDeadline(time.Now().Add(wait)); err != nil {
					return fmt.Errorf("set read deadline: %w", err)
				}
			}

			fmt.Printf("listening on %s\n", listen)
			buf := make([]byte, 65536)
			seen := 0
			for {
				n, from, err := conn.ReadFromUDP(buf)
				if err != nil {
					return fmt.Errorf("read udp: %w", err)
				}

				frame := append([]byte(nil), buf[:n]...)
				fmt.Printf("from=%s bytes=%d %s\n", from.String(), len(frame), formatFrame(frame))
				seen++
				if count > 0 && seen >= count {
					return nil
				}
			}
		},
	}

	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:11002", "UDP address to listen on")
	cmd.Flags().IntVar(&count, "count", 0, "stop after receiving this many frames")
	cmd.Flags().DurationVar(&wait, "wait", 0, "stop waiting after this duration")

	return cmd
}

func formatFrame(frame []byte) string {
	if len(frame) < 14 {
		return fmt.Sprintf("short-frame hex=%s", hex.EncodeToString(frame))
	}

	dst := net.HardwareAddr(frame[0:6]).String()
	src := net.HardwareAddr(frame[6:12]).String()
	etherTy := uint16(frame[12])<<8 | uint16(frame[13])
	payload := string(frame[14:])

	return fmt.Sprintf(
		"dst=%s src=%s ethertype=0x%04x payload=%q hex=%s",
		dst,
		src,
		etherTy,
		payload,
		strings.ToLower(hex.EncodeToString(frame)),
	)
}
