package integration

import (
	"errors"
	"net/netip"
)

func AllocateIP(
	subnet netip.Prefix,
	exclusions []Range[netip.Addr],
	allocatedStr []string,
) (string, error) {
	allocated, err := MapSliceErr(allocatedStr, netip.ParseAddr)
	if err != nil {
		return "", errors.New("failed to convert allocated IPs from strings")
	}

	allocatedSet := make(map[netip.Addr]struct{}, len(allocated))
	for _, ip := range allocated {
		allocatedSet[ip] = struct{}{}
	}

	// Iterate over all usable IPs in subnet
	for ip := subnet.Addr().Next(); subnet.Contains(ip); ip = ip.Next() {
		// Skip excluded range
		excluded := false
		for _, exclusion := range exclusions {
			if ip.Compare(exclusion.Start) >= 0 && ip.Compare(exclusion.End) <= 0 {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		// Skip already allocated IPs
		if _, exists := allocatedSet[ip]; exists {
			continue
		}

		return ip.String(), nil
	}

	return "", errors.New("no free IPs available")
}
