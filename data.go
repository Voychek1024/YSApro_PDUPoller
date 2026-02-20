package main

const NoEnvData = "--.-"

type loginConf struct {
	PduIP      string
	PduUserB64 string
	PduPassB64 string
}
type loginPayload struct {
	Login    int    `json:"login"`
	UserName string `json:"userName"`
	PassWord string `json:"password"`
}

type loginResult struct {
	LoginVerif int `json:"login_verif"`
	LoginCnt   int `json:"login_cnt"`
}

type dataResult struct {
	SockNums   int      `json:"sock_nums"`
	Vol        string   `json:"vol"`
	TolCur     string   `json:"tolCur"`
	TolPow     string   `json:"tolPow"`
	TolEne     string   `json:"tolEne"`
	Fre        string   `json:"fre"`
	TolCos     string   `json:"tolCos"`
	TempCent   string   `json:"tempCent"`
	TempFahr   string   `json:"tempFahr"`
	Hum        string   `json:"hum"`
	SockName   []string `json:"sock_name"`
	Socket     []int    `json:"socket"`
	SockCtlSta []int    `json:"sockctl_sta"`
}

type deviceInfo struct {
	DeviceName    string `json:"dev_name"`
	ProductModel  string `json:"pro_model"`
	DeviceModel   string `json:"dev_model"`
	DeviceID      string `json:"dev_id"`
	DeviceMAC     string `json:"dev_mac"`
	DeviceVersion string `json:"dev_ver"`
	DeviceUptime  int    `json:"uptime"`
	ManYear       int    `json:"year"`
	ManMon        int    `json:"mon"`
	ManDay        int    `json:"day"`
}

var metricLabel = []string{
	"device_name",
	"product_model",
	"device_model",
	"device_id",
	"device_mac",
	"device_version",
}
