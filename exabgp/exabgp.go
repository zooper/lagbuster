package exabgp

import (
	"encoding/json"
	"fmt"
	"os"
)

// PipePath is the path to the named pipe for ExaBGP communication
const PipePath = "/var/run/lagbuster/exabgp.pipe"

// Client represents an ExaBGP API client
type Client struct {
	pipePath string
}

// Command represents a command to send to ExaBGP
type Command struct {
	Action   string   `json:"action"`   // "announce" or "withdraw"
	Prefixes []string `json:"prefixes"` // List of IPv6 prefixes
	Peer     string   `json:"peer"`     // Peer name (edgenyc01, ash01)
	Priority int      `json:"priority"` // 1 (healthy) or 99 (unhealthy)
	NextHop  string   `json:"nexthop"`  // Next-hop IPv6 address
}

// NewClient creates a new ExaBGP client
func NewClient() *Client {
	return &Client{
		pipePath: PipePath,
	}
}

// NewClientWithPipe creates a client with a custom pipe path (for testing)
func NewClientWithPipe(pipePath string) *Client {
	return &Client{
		pipePath: pipePath,
	}
}

// SendCommand sends a command to ExaBGP via the named pipe
func (c *Client) SendCommand(cmd Command) error {
	// Convert command to JSON
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	// Open named pipe for writing
	pipe, err := os.OpenFile(c.pipePath, os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open pipe %s: %w", c.pipePath, err)
	}
	defer pipe.Close()

	// Write command
	_, err = pipe.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write to pipe: %w", err)
	}

	return nil
}

// AnnounceRoutes announces routes for a peer with the given priority
func (c *Client) AnnounceRoutes(peer string, nexthop string, prefixes []string, priority int) error {
	cmd := Command{
		Action:   "announce",
		Prefixes: prefixes,
		Peer:     peer,
		Priority: priority,
		NextHop:  nexthop,
	}

	return c.SendCommand(cmd)
}

// WithdrawRoutes withdraws routes for a peer
func (c *Client) WithdrawRoutes(peer string, nexthop string, prefixes []string) error {
	cmd := Command{
		Action:   "withdraw",
		Prefixes: prefixes,
		Peer:     peer,
		NextHop:  nexthop,
	}

	return c.SendCommand(cmd)
}

// SetPeerPriority sets the priority for a peer (announces all prefixes with new priority)
func (c *Client) SetPeerPriority(peer string, nexthop string, prefixes []string, priority int) error {
	// ExaBGP will update the route if we re-announce with different attributes
	return c.AnnounceRoutes(peer, nexthop, prefixes, priority)
}
