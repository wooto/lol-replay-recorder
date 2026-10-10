package lcu

// replayPlatform maps client region labels to the platform prefix used by ROFL files.
func replayPlatform(region string) string {
	switch region {
	case "NA":
		return "NA1"
	case "EUW":
		return "EUW1"
	case "EUNE":
		return "EUN1"
	case "BR":
		return "BR1"
	case "LAN":
		return "LA1"
	case "LAS":
		return "LA2"
	case "OCE":
		return "OC1"
	case "TR":
		return "TR1"
	case "JP":
		return "JP1"
	case "VN":
		return "VN2"
	default:
		return region
	}
}
