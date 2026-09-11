package config

// Device describes a Flydigi controller model as identified by its device ID.
type Device struct {
	// Code is Flydigi's short model code ("f5", "k5", ...), used by the firmware update API.
	Code string
	// Name is a human readable model name.
	Name string
}

// Devices maps Flydigi device IDs to model information. IDs and codes come from
// Flydigi Space Station 4 (DeviceType enum); names from Space Station 3 tables.
var Devices = map[int32]Device{
	18:  {"apex", "Apex"},
	19:  {"apex2", "Apex 2"},
	20:  {"f1", "Vader 2"},
	21:  {"f1_l", "Vader 2"},
	22:  {"f1p", "Vader 2 Pro"},
	23:  {"f1", "Vader 2"},
	24:  {"k1", "Apex 3"},
	26:  {"k1", "Apex 3 (Aerospace)"},
	29:  {"k1", "Apex 3 (One Piece)"},
	25:  {"fp1", "Direwolf"},
	27:  {"fp1", "Direwolf (wired)"},
	30:  {"fp1", "Direwolf (Fate)"},
	31:  {"fp1s", "Direwolf S"},
	28:  {"f3", "Vader 3"},
	80:  {"f3p", "Vader 3 Pro"},
	81:  {"f3p", "Vader 3 Pro (One Piece)"},
	88:  {"f3p", "Vader 3 Pro (EVA)"},
	82:  {"fp2", "Direwolf 2"},
	83:  {"fp2", "Direwolf 2 (Naruto)"},
	89:  {"fp2", "Direwolf 2 (wired)"},
	90:  {"fp2", "Direwolf 2 (Switch)"},
	94:  {"fp2", "Direwolf 2 M"},
	84:  {"k2", "Apex 4"},
	86:  {"k2", "Apex 4 (EVA)"},
	87:  {"k2", "Apex 4 (STN)"},
	92:  {"k2", "Apex 4 (Assassin's Creed)"},
	93:  {"k2", "Apex 4 (Russia)"},
	102: {"k2", "Apex 4 (Black Myth: Wukong)"},
	103: {"k2", "Apex 4 (Yae Miko)"},
	104: {"k2", "Apex 4 (Firefly)"},
	85:  {"f4", "Vader 4 Pro"},
	91:  {"f4", "Vader 4 Pro (Assassin's Creed)"},
	105: {"f4", "Vader 4 Pro (JL)"},
	95:  {"fp3", "Direwolf 3"},
	97:  {"fp3", "Direwolf 3 Pro (Naruto)"},
	98:  {"fp3", "Direwolf 3 (wired)"},
	99:  {"fp3", "Direwolf 3 (Switch)"},
	100: {"fp3", "Direwolf 3 (Naruto)"},
	128: {"k5", "Apex 5"},
	129: {"k5", "Apex 5 (EVA)"},
	133: {"k5", "Apex 5 (MM)"},
	134: {"k5", "Apex 5 (SRS)"},
	135: {"k5", "Apex 5 (GS)"},
	136: {"k5", "Apex 5 (LZ)"},
	130: {"f5", "Vader 5 Pro"},
	144: {"f5", "Vader 5 Pro (Dragon Ball Z)"},
	145: {"f5", "Vader 5 Pro (HK3)"},
	132: {"fp4", "Direwolf 4"},
	146: {"fp4", "Direwolf 4 (GS)"},
	147: {"fp4", "Direwolf 4 (JDB)"},
	148: {"fp4", "Direwolf 4 (MRFZ)"},
	149: {"k6", "Apex 6"},
	150: {"k6", "Apex 6 Pro"},
}

// LookupDevice returns the model information for a device ID, falling back to
// a generic entry for unknown IDs.
func LookupDevice(id int32) Device {
	if d, ok := Devices[id]; ok {
		return d
	}
	return Device{Code: "", Name: "Unknown"}
}
