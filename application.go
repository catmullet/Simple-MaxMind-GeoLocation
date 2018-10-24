package main

import (
	"fmt"
	"net/http"
	"encoding/csv"
	"bufio"
	"archive/zip"
	"strings"
	"strconv"
	"os"
	"io"
	"path/filepath"
	"encoding/json"
)

const GEOIP_BASE = "geo_tmp"
const GEOIP_FILENAME = "/tmp_csv.zip"

var Blocks map[string]Country

type Country struct {
	IsoCode     string `json:"iso_code"`
	CountryName string `json:"country_name"`
	Subdivision string `json:"subdivision"`
	CityName 	string `json:"city_name"`
	TimeZone	string `json:"time_zone"`
}

type UpdateResponse struct {
	Status string `json:"status"`
	TotalIPs int `json:"total_ips"`
}

var IPMap map[string]Country
var IPMapList map[string][]string
var TmpCountryMap map[string]Country

func main() {

	go GetUpdate()

	http.HandleFunc("/ip", func (w http.ResponseWriter, r *http.Request) {
		values := r.URL.Query()
		ip := values.Get("address")

		w.Header().Set("Content-Type", "application/json")

		country, err := GetCountryByIP(ip)

		countryJson, err := json.Marshal(country)

		if err != nil {
			fmt.Fprintf(w, "Failed To Retrieve Country")
		}

		fmt.Fprintf(w, string(countryJson))

	})

	http.HandleFunc("/update", func (w http.ResponseWriter, r *http.Request) {

		go GetUpdate()

		w.Header().Set("Content-Type", "application/json")

		response := UpdateResponse{Status:"OK", TotalIPs: len(IPMapList)}

		resp, _ := json.Marshal(response)

		fmt.Fprintf(w, string(resp))

	})

	http.HandleFunc("/health", func (w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Content-Type", "application/json")

		response := UpdateResponse{Status:"OK", TotalIPs: len(IPMapList)}

		resp, _ := json.Marshal(response)

		fmt.Fprintf(w, string(resp))

	})

	http.ListenAndServe(":5000", nil)

}

func GetCountryByIP(ip string) (Country, error) {

	strippedIp := StripIP(ip, 1)
	previousKey := ""

	firstNumber := strings.Split(ip, ".")[0]

	if list, ok := IPMapList[firstNumber]; ok {
		for _,val := range list {
			if isIPGreater(strings.Split(val, "/")[0], strippedIp) {
				fmt.Println(val)
				return IPMap[previousKey], nil
			} else {
				previousKey = val
			}
		}
	}

	return IPMap[previousKey], nil
}

func isIPGreater(ip string, ipCompare string) bool {
	ipSlice := strings.Split(ip, ".")
	ipCompareSlice := strings.Split(ipCompare, ".")

	for idx, val := range ipSlice {
		ipValue,_ := strconv.Atoi(val)
		ipCompareValue,_ := strconv.Atoi(ipCompareSlice[idx])
		if ipValue == ipCompareValue {
			continue;
		}
		if ipValue > ipCompareValue {
			return true
		} else {
			return false
		}
	}
	return false
}

func StripIP(ip string, position int) string {
	ipAddressSlice := strings.Split(ip, ".")

	ipAddressSlice = ipAddressSlice[:len(ipAddressSlice) - position]
	for i := 0; i < position; i++ {
		ipAddressSlice = append(ipAddressSlice, "0")
	}

	return strings.Join(ipAddressSlice, ".")
}

func GetUpdate() {
	RemoveTempFiles()

	err := DownloadFile()
	if err != nil {
		fmt.Println(err)
	}

	files, err := Unzip(GEOIP_BASE+GEOIP_FILENAME, GEOIP_BASE)
	if err != nil {
		fmt.Println(err)
	}
	if len(files) == 0 {
		fmt.Println("No Files in Zip or Unzip errored out")
	}

	for _, filename := range files {
		if strings.Contains(filename, "Locations-en") {
			ParseCountries(filename)
		}
	}

	for _, filename := range files {
		if strings.Contains(filename, "Blocks-IPv4") {
			ParseBlocks(filename)
		}
	}
}

func RemoveTempFiles(){
	err := os.RemoveAll(GEOIP_BASE)

	if err != nil {
		fmt.Println(err)
	}

	err = os.MkdirAll(GEOIP_BASE, 0755)

	if err != nil {
		fmt.Println(err)
	}
}

func ParseCountries(filePath string) {
	TmpCountryMap = make(map[string]Country)

	csvFile, _ := os.Open(filePath)
	reader := csv.NewReader(bufio.NewReader(csvFile))
	for {
		line, error := reader.Read()
		if error == io.EOF {
			break
		} else if error != nil {
			fmt.Println(error)
		}

		if line[4] == "country_iso" {
			break
		}

		if line[4] == "" || line[5] == "" {
			TmpCountryMap[line[0]] = Country{CountryName: line[3], IsoCode: line[2], CityName: line[10], Subdivision:line[7], TimeZone:line[12]}
		} else {
			TmpCountryMap[line[0]] = Country{CountryName: line[5], IsoCode: line[4], CityName: line[10], Subdivision:line[7], TimeZone:line[12]}
		}

		fmt.Println(TmpCountryMap[line[0]])
	}
}

func ParseBlocks(filePath string) {
	csvFile, _ := os.Open(filePath)
	reader := csv.NewReader(bufio.NewReader(csvFile))
	IPMap = make(map[string]Country)
	IPMapList = make(map[string][]string)

	for {
		line, error := reader.Read()
		if error == io.EOF {
			break
		} else if error != nil {
			fmt.Println(error)
			break
		}

		if len(line) < 2 && line[0] == "network" {
			continue
		}

		if strings.Contains(line[0], "/") {
			fmt.Printf("\r %s", line[0])

			firstNumber := strings.Split(line[0], ".")[0]

			if _, ok := IPMapList[firstNumber]; ok {
				IPMapList[firstNumber] = append(IPMapList[firstNumber], line[0])
			} else {
				IPMapList[firstNumber] = []string{line[0]}
			}

			IPMap[line[0]] = TmpCountryMap[line[1]]
		}
	}
}
func DownloadFile() error {

	url := "http://geolite.maxmind.com/download/geoip/database/GeoLite2-City-CSV.zip" //os.Getenv("GEOIP_CSV_LINK")
	filepath := GEOIP_BASE + GEOIP_FILENAME

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

// Unzip will decompress a zip archive, moving all files and folders
// within the zip file (parameter 1) to an output directory (parameter 2).
func Unzip(src string, dest string) ([]string, error) {

	var filenames []string

	r, err := zip.OpenReader(src)
	if err != nil {
		return filenames, err
	}
	defer r.Close()

	for _, f := range r.File {

		rc, err := f.Open()
		if err != nil {
			return filenames, err
		}
		defer rc.Close()

		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return filenames, fmt.Errorf("%s: illegal file path", fpath)
		}

		filenames = append(filenames, fpath)

		if f.FileInfo().IsDir() {

			// Make Folder
			os.MkdirAll(fpath, os.ModePerm)

		} else {

			// Make File
			if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
				return filenames, err
			}

			outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return filenames, err
			}

			_, err = io.Copy(outFile, rc)

			// Close the file without defer to close before next iteration of loop
			outFile.Close()

			if err != nil {
				return filenames, err
			}

		}
	}
	return filenames, nil
}

