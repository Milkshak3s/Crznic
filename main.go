package main

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"log"
	"net"
	"os"
	"strconv"
	"time"
	"errors"
)

// get the local ip and port based on our destination ip
func localIPPort(dstip net.IP) (net.IP, layers.TCPPort) {
	serverAddr, err := net.ResolveUDPAddr("udp", dstip.String()+":12345")
	if err != nil {
		log.Fatal(err)
	}

	// We don't actually connect to anything, but we can determine
	// based on our destination ip what source ip we should use.
	if con, err := net.DialUDP("udp", nil, serverAddr); err == nil {
		if udpaddr, ok := con.LocalAddr().(*net.UDPAddr); ok {
			tcpPort := layers.TCPPort(udpaddr.Port)
			return udpaddr.IP, tcpPort
		}
	}
	log.Fatal("could not get local ip: " + err.Error())
	return nil, layers.TCPPort(1)
}

// reads the reply on a connection
func readReply(conn net.PacketConn, dstip net.IP, dstport layers.TCPPort, srcport layers.TCPPort) (error) {
	for {
		b := make([]byte, 4096)
		log.Println("reading from conn")
		n, addr, err := conn.ReadFrom(b)
		if err != nil {
			return err
		} else if addr.String() == dstip.String() {
			// Decode a packet
			packet := gopacket.NewPacket(b[:n], layers.LayerTypeTCP, gopacket.Default)
			// Get the TCP layer from this packet
			if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
				tcp, _ := tcpLayer.(*layers.TCP)

				if tcp.DstPort == srcport {
					if tcp.SYN && tcp.ACK {
						log.Printf("Recieved syn/ack: %s\n", dstport)
						return nil
					} else {
						return errors.New("did not receive syn/ack")
					}
				}
			}
		} else {
			return errors.New("got packet not matching addr")
		}
	}
}

// builds and sends a tcp packet
func sendSyn(dstip net.IP, dstport layers.TCPPort, seq uint32) (error) {
	srcip, srcport := localIPPort(dstip)
	log.Printf("using srcip: %v", srcip.String())

	// Our IP header... not used, but necessary for TCP checksumming.
	ip := &layers.IPv4{
		SrcIP:    srcip,
		DstIP:    dstip,
		Protocol: layers.IPProtocolTCP,
	}

	// Our TCP header
	tcp := &layers.TCP{
		SrcPort: srcport,
		DstPort: dstport,
		Seq:     seq,
		SYN:     true,
		Window:  14600,
	}
	tcp.SetNetworkLayerForChecksum(ip)

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	if err := gopacket.SerializeLayers(buf, opts, tcp); err != nil {
		return err
	}

	conn, err := net.ListenPacket("ip4:tcp", "0.0.0.0")
	if err != nil {
		return err
	}
	defer conn.Close()
	log.Println("writing request")
	if _, err := conn.WriteTo(buf.Bytes(), &net.IPAddr{IP: dstip}); err != nil {
		return err
	}

	// Set deadline so we don't wait forever.
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}

	// read the reply
	if err := readReply(conn, dstip, dstport, srcport); err != nil {
		return err
	}

	return nil
}

func main() {
	if len(os.Args) != 3 {
		log.Printf("Usage: %s <host/ip> <port>\n", os.Args[0])
		os.Exit(-1)
	}
	log.Println("starting")

	// define the seq for this interaction
	var seq uint32
	seq = 2132141

	dstaddrs, err := net.LookupIP(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	// parse the destination host and port from the command line os.Args
	dstip := dstaddrs[0].To4()
	var dstport layers.TCPPort
	if d, err := strconv.ParseUint(os.Args[2], 10, 16); err != nil {
		log.Fatal(err)
	} else {
		dstport = layers.TCPPort(d)
	}

	// send the first syn packet
	err = sendSyn(dstip, dstport, seq)
	if err != nil {
		log.Fatal(err)
	}
}
