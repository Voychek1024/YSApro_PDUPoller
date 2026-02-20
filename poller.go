package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/natefinch/lumberjack"
	log "github.com/sirupsen/logrus"

	"github.com/spf13/viper"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var sockGauge *prometheus.GaugeVec
var volGauge *prometheus.GaugeVec
var curGauge *prometheus.GaugeVec
var powGauge *prometheus.GaugeVec
var eneGauge *prometheus.GaugeVec
var freGauge *prometheus.GaugeVec
var cosGauge *prometheus.GaugeVec
var tempCentGauge *prometheus.GaugeVec
var tempFahrGauge *prometheus.GaugeVec
var humGauge *prometheus.GaugeVec
var socketStaGauge *prometheus.GaugeVec
var uptimeGauge *prometheus.GaugeVec

func init() {
	log.SetFormatter(&log.TextFormatter{})
	log.SetLevel(log.InfoLevel)
	log.SetOutput(&lumberjack.Logger{
		Filename:   "/var/log/pdu_poller.log",
		MaxSize:    10,
		MaxAge:     30,
		MaxBackups: 3,
	})
}

func pollerWorker(pduIP, userName, password string, delay time.Duration, doneChan chan bool) {
	// service url
	var loginUrl, dataUrl, aboutUrl string
	loginUrl = fmt.Sprintf("http://%s/login.json", pduIP)
	dataUrl = fmt.Sprintf("http://%s/home-page.json", pduIP)
	aboutUrl = fmt.Sprintf("http://%s/about.json", pduIP)

	cli := &http.Client{
		Timeout: 1 * time.Second,
	}

	// login device
	loginReq := &loginPayload{
		Login:    255, // fixed
		UserName: userName,
		PassWord: password,
	}
	mLoginReq, err := json.Marshal(loginReq)
	if err != nil {
		close(doneChan)
		log.Fatalf("json.Marshal failed: %v", err)
	}
	respLogin, err := cli.Post(loginUrl, "application/json", bytes.NewBuffer(mLoginReq))
	if err != nil {
		close(doneChan)
		log.Fatalf("<login> cli.Post failed: %v", err)
	}
	defer func() {
		_ = respLogin.Body.Close()
	}()
	if respLogin.StatusCode == http.StatusOK {
		loginRes := &loginResult{}
		resp := make([]byte, 0)
		resp, err = io.ReadAll(respLogin.Body)
		if err != nil {
			close(doneChan)
			log.Fatalf("<login> io.ReadAll failed: %v", err)
		}
		err = json.Unmarshal(resp, loginRes)
		if err != nil {
			close(doneChan)
			log.Fatalf("<login> json.Unmarshal failed: %v", err)
		}
		if loginRes.LoginVerif == 3 {
			log.Infof("<login> pdu(%s) login successful.", pduIP)
		} else {
			close(doneChan)
			log.Fatalf("<login> login failed, LoginResp: %d != 3", loginRes.LoginVerif)
		}
	} else {
		close(doneChan)
		log.Fatalf("<login> login failed, StatusCode: %d != 200", respLogin.StatusCode)
	}

	// get device info
	devDetail := &deviceInfo{}
	ticker := time.NewTicker(delay)
	for {
		select {
		case <-doneChan:
			log.Warnf("pollerWorker thread %s Graceful Shutdown...", pduIP)
			return
		case <-ticker.C:
			respAbout, errGet := cli.Post(aboutUrl, "application/json", bytes.NewBuffer([]byte(`{"get":255}`)))
			if errGet != nil {
				close(doneChan)
				log.Fatalf("<about> cli.Post failed: %v", errGet)
			}
			if respAbout.StatusCode == http.StatusOK {
				resp := make([]byte, 0)
				resp, errGet = io.ReadAll(respAbout.Body)
				if errGet != nil {
					close(doneChan)
					log.Fatalf("<about> io.ReadAll failed: %v", errGet)
				}
				errGet = json.Unmarshal(resp, devDetail)
				if errGet != nil {
					close(doneChan)
					log.Fatalf("<about> json.Unmarshal failed: %v", errGet)
				}
				_ = respAbout.Body.Close()
				uptimeGauge.WithLabelValues(
					devDetail.DeviceName,
					devDetail.ProductModel,
					devDetail.DeviceModel,
					devDetail.DeviceID,
					devDetail.DeviceMAC,
					devDetail.DeviceVersion,
				).Set(float64(devDetail.DeviceUptime))
			} else {
				close(doneChan)
				log.Fatalf("<about> get failed, StatusCode: %d != 200", respAbout.StatusCode)
			}

			respData, errGet := cli.Post(dataUrl, "application/json", bytes.NewBuffer([]byte(`{"get":255}`)))
			if errGet != nil {
				close(doneChan)
				log.Fatalf("<about> cli.Post failed: %v", errGet)
			}

			if respData.StatusCode == http.StatusOK {
				data := &dataResult{}
				resp := make([]byte, 0)
				resp, errGet = io.ReadAll(respData.Body)
				if errGet != nil {
					close(doneChan)
					log.Fatalf("<data> io.ReadAll failed: %v", errGet)
				}
				errGet = json.Unmarshal(resp, data)
				if errGet != nil {
					close(doneChan)
					log.Fatalf("<data> json.Unmarshal failed: %v", errGet)
				}
				_ = respData.Body.Close()
				sockGauge.WithLabelValues(
					devDetail.DeviceName,
					devDetail.ProductModel,
					devDetail.DeviceModel,
					devDetail.DeviceID,
					devDetail.DeviceMAC,
					devDetail.DeviceVersion,
				).Set(float64(data.SockNums))
				fVol, errParse := strconv.ParseFloat(data.Vol, 64)
				if errParse != nil {
					close(doneChan)
					log.Fatalf("<data> vol strconv.ParseFloat failed: %v", errParse)
				}
				volGauge.WithLabelValues(
					devDetail.DeviceName,
					devDetail.ProductModel,
					devDetail.DeviceModel,
					devDetail.DeviceID,
					devDetail.DeviceMAC,
					devDetail.DeviceVersion,
				).Set(fVol)
				fTolCur, errParse := strconv.ParseFloat(data.TolCur, 64)
				if errParse != nil {
					close(doneChan)
					log.Fatalf("<data> tolCur strconv.ParseFloat failed: %v", errParse)
				}
				curGauge.WithLabelValues(
					devDetail.DeviceName,
					devDetail.ProductModel,
					devDetail.DeviceModel,
					devDetail.DeviceID,
					devDetail.DeviceMAC,
					devDetail.DeviceVersion,
				).Set(fTolCur)
				fTolPow, errParse := strconv.ParseFloat(data.TolPow, 64)
				if errParse != nil {
					close(doneChan)
					log.Fatalf("<data> tolPow strconv.ParseFloat failed: %v", errParse)
				}
				powGauge.WithLabelValues(
					devDetail.DeviceName,
					devDetail.ProductModel,
					devDetail.DeviceModel,
					devDetail.DeviceID,
					devDetail.DeviceMAC,
					devDetail.DeviceVersion,
				).Set(fTolPow)
				fTolEne, errParse := strconv.ParseFloat(data.TolEne, 64)
				if errParse != nil {
					close(doneChan)
					log.Fatalf("<data> tolEne strconv.ParseFloat failed: %v", errParse)
				}
				eneGauge.WithLabelValues(
					devDetail.DeviceName,
					devDetail.ProductModel,
					devDetail.DeviceModel,
					devDetail.DeviceID,
					devDetail.DeviceMAC,
					devDetail.DeviceVersion,
				).Set(fTolEne)
				fFre, errParse := strconv.ParseFloat(data.Fre, 64)
				if errParse != nil {
					close(doneChan)
					log.Fatalf("<data> fre strconv.ParseFloat failed: %v", errParse)
				}
				freGauge.WithLabelValues(
					devDetail.DeviceName,
					devDetail.ProductModel,
					devDetail.DeviceModel,
					devDetail.DeviceID,
					devDetail.DeviceMAC,
					devDetail.DeviceVersion,
				).Set(fFre)
				fTolCos, errParse := strconv.ParseFloat(data.TolCos, 64)
				if errParse != nil {
					close(doneChan)
					log.Fatalf("<data> tolCos strconv.ParseFloat failed: %v", errParse)
				}
				cosGauge.WithLabelValues(
					devDetail.DeviceName,
					devDetail.ProductModel,
					devDetail.DeviceModel,
					devDetail.DeviceID,
					devDetail.DeviceMAC,
					devDetail.DeviceVersion,
				).Set(fTolCos)
				if data.TempCent != NoEnvData {
					fTempCent, errParse := strconv.ParseFloat(data.TempCent, 64)
					if errParse != nil {
						close(doneChan)
						log.Fatalf("<data> tempCent strconv.ParseFloat failed: %v", errParse)
					}
					tempCentGauge.WithLabelValues(
						devDetail.DeviceName,
						devDetail.ProductModel,
						devDetail.DeviceModel,
						devDetail.DeviceID,
						devDetail.DeviceMAC,
						devDetail.DeviceVersion,
					).Set(fTempCent)
				}
				if data.TempFahr != NoEnvData {
					fTempFahr, errParse := strconv.ParseFloat(data.TempFahr, 64)
					if errParse != nil {
						close(doneChan)
						log.Fatalf("<data> tempFahr strconv.ParseFloat failed: %v", errParse)
					}
					tempFahrGauge.WithLabelValues(
						devDetail.DeviceName,
						devDetail.ProductModel,
						devDetail.DeviceModel,
						devDetail.DeviceID,
						devDetail.DeviceMAC,
						devDetail.DeviceVersion,
					).Set(fTempFahr)
				}
				if data.Hum != NoEnvData {
					fHum, errParse := strconv.ParseFloat(data.Hum, 64)
					if errParse != nil {
						close(doneChan)
						log.Fatalf("<data> hum strconv.ParseFloat failed: %v", errParse)
					}
					humGauge.WithLabelValues(
						devDetail.DeviceName,
						devDetail.ProductModel,
						devDetail.DeviceModel,
						devDetail.DeviceID,
						devDetail.DeviceMAC,
						devDetail.DeviceVersion,
					).Set(fHum)
				}
				for i, name := range data.SockName {
					socketStaGauge.WithLabelValues(
						devDetail.DeviceName,
						devDetail.ProductModel,
						devDetail.DeviceModel,
						devDetail.DeviceID,
						devDetail.DeviceMAC,
						devDetail.DeviceVersion,
						name,
					).Set(float64(data.Socket[i]))
				}
			} else {
				close(doneChan)
				log.Fatalf("<data> get failed, StatusCode: %d != 200", respData.StatusCode)
			}
		}
	}
}

func main() {
	// read conf
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("viper parse config failed: %v", err)
	}
	pduInfoSt := make([]*loginConf, 0)
	if pduInfoItf := viper.Get("pdu_info"); pduInfoItf != nil {
		pduInfoS := pduInfoItf.([]interface{})
		for _, info := range pduInfoS {
			infoM := info.(map[string]interface{})
			infoC := &loginConf{
				PduIP:      infoM["ip"].(string),
				PduUserB64: base64.StdEncoding.EncodeToString([]byte(infoM["user"].(string))),
				PduPassB64: base64.StdEncoding.EncodeToString([]byte(infoM["pass"].(string))),
			}
			pduInfoSt = append(pduInfoSt, infoC)
		}
	} else {
		log.Fatalf("not configured pdu_info, exit.")
	}
	promPort := viper.GetInt("scrape.port")
	if promPort == 0 {
		log.Fatalf("not configured scrape.port, exit.")
	}
	interval := viper.GetDuration("scrape.interval")
	if time.Duration(0) <= interval || 1*time.Minute >= interval {
		interval = 10 * time.Second
	}

	// metrics
	sockGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pdu_total_sockets",
	}, metricLabel)
	volGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pdu_total_voltage",
	}, metricLabel)
	curGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pdu_total_current",
	}, metricLabel)
	powGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pdu_total_power",
	}, metricLabel)
	eneGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pdu_total_energy",
	}, metricLabel)
	freGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pdu_total_frequency",
	}, metricLabel)
	cosGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pdu_total_cost",
	}, metricLabel)
	tempCentGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pdu_environment_temp_Cent",
	}, metricLabel)
	tempFahrGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pdu_environment_temp_Fahr",
	}, metricLabel)
	humGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pdu_environment_humidity",
	}, metricLabel)
	socketStaGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pdu_socket_status",
	}, append(metricLabel, "socket_name"))
	uptimeGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pdu_total_uptime",
	}, metricLabel)

	// prometheus
	promUrl := fmt.Sprintf(":%d", promPort)
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		sockGauge,
		volGauge,
		curGauge,
		powGauge,
		eneGauge,
		freGauge,
		cosGauge,
		tempCentGauge,
		tempFahrGauge,
		humGauge,
		socketStaGauge,
		uptimeGauge,
	)
	promSrv := &http.Server{Addr: promUrl}
	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	go func() {
		errSrv := promSrv.ListenAndServe()
		if errSrv != nil && !errors.Is(errSrv, http.ErrServerClosed) {
			log.Fatalf("http.ListenAndServe failed: %v", errSrv)
		}
		log.Warnf("http.ListenAndServe: %v", errSrv)
	}()

	sig := make(chan os.Signal, 1)
	done := make(chan bool)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		{
			log.Warnf("Graceful Shutdown...")
			close(done)
		}
	}()

	var wg sync.WaitGroup
	for _, info := range pduInfoSt {
		wg.Add(1)
		go func() {
			pollerWorker(info.PduIP, info.PduUserB64, info.PduPassB64, interval, done)
			wg.Done()
		}()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		select {
		case <-done:
			wg.Wait()
			_ = promSrv.Shutdown(ctx)
			return
		}
	}
}
