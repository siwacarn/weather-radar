package main

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"net/http"
	"os"
	"time"

	"gocv.io/x/gocv"
)

type RadarIntensity struct {
	Min [3]int
	Max [3]int
}

const (
	RadarImgURL = "https://weather.bangkok.go.th/Images/Radar/radar.jpg"
	// RadarImgURL     = "https://weather.bangkok.go.th/Images/Radar/radarh.jpg"
	ImgPath         = "radar_img/"
	RefreshInterval = 5 // in minutes
)

var (
	RadarIntensityLow  = RadarIntensity{Min: [3]int{40, 100, 100}, Max: [3]int{70, 255, 255}}  // 9.5 - 29.0 dBz
	RadarIntensityMid  = RadarIntensity{Min: [3]int{16, 100, 100}, Max: [3]int{39, 255, 255}}  // 29.0 - 44.0 dBz
	RadarIntensityHigh = RadarIntensity{Min: [3]int{140, 100, 100}, Max: [3]int{15, 255, 255}} // 44.0+ dBz

	// Cropping coordinates
	startX = 240
	startY = 360
	endX   = startX + 300
	endY   = startY + 200

	// startX = 580
	// endX   = startX + 440
	// startY = 700
	// endY   = startY + 350
)

func main() {
	var err error

	startTime := time.Now()

	// Ensure the image directory exists
	if err = os.MkdirAll(ImgPath, os.ModePerm); err != nil {
		fmt.Printf("Error creating image directory: %v\n", err)
		return
	}

	// Run the process once
	if cloudsPercent, err := getCloudsPercentage(err == nil); err != nil {
		fmt.Printf("Error getting clouds percentage: %v\n", err)
	} else {
		fmt.Printf("Final Clouds percentage: %.1f%%\n", cloudsPercent)
	}

	endTime := time.Now()

	// Calculate the time taken to run the process
	elapsedTime := endTime.Sub(startTime)
	fmt.Printf("Time taken: %v\n", elapsedTime)
}

func getRadarImage() (gocv.Mat, error) {
	// Get radar image
	resp, err := http.Get(RadarImgURL)
	if err != nil {
		return gocv.NewMat(), err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return gocv.NewMat(), err
	}

	img, err := gocv.IMDecode(body, gocv.IMReadColor)
	if err != nil {
		return gocv.NewMat(), err
	}

	if img.Empty() {
		return gocv.NewMat(), fmt.Errorf("decoded image is empty")
	}

	return img, nil
}

func getDBzMask(img gocv.Mat, radarIntensity RadarIntensity) (gocv.Mat, error) {
	hsv := gocv.NewMat()
	defer hsv.Close()
	gocv.CvtColor(img, &hsv, gocv.ColorBGRToHSV)

	var mask gocv.Mat

	// check for hua wrap-around
	if radarIntensity.Min[0] > radarIntensity.Max[0] {
		lower1 := gocv.NewScalar(float64(radarIntensity.Min[0]), float64(radarIntensity.Min[1]), float64(radarIntensity.Min[2]), 0)
		upper1 := gocv.NewScalar(180, float64(radarIntensity.Max[1]), float64(radarIntensity.Max[2]), 0)
		mask1 := gocv.NewMat()
		gocv.InRangeWithScalar(hsv, lower1, upper1, &mask1)

		lower2 := gocv.NewScalar(0, float64(radarIntensity.Min[1]), float64(radarIntensity.Min[2]), 0)
		upper2 := gocv.NewScalar(float64(radarIntensity.Max[0]), float64(radarIntensity.Max[1]), float64(radarIntensity.Max[2]), 0)
		mask2 := gocv.NewMat()
		gocv.InRangeWithScalar(hsv, lower2, upper2, &mask2)

		mask = gocv.NewMat()
		gocv.Add(mask1, mask2, &mask)
		mask1.Close()
		mask2.Close()
	} else {
		lower := gocv.NewScalar(float64(radarIntensity.Min[0]), float64(radarIntensity.Min[1]), float64(radarIntensity.Min[2]), 0)
		upper := gocv.NewScalar(float64(radarIntensity.Max[0]), float64(radarIntensity.Max[1]), float64(radarIntensity.Max[2]), 0)
		mask = gocv.NewMat()
		gocv.InRangeWithScalar(hsv, lower, upper, &mask)
	}

	return mask, nil
}

func getWhitePercentage(img gocv.Mat) float64 {

	width := img.Cols()
	fmt.Println("Width: ", width)
	height := img.Rows()
	fmt.Println("Height: ", height)
	size := float64(width * height)
	whiteArea := float64(gocv.CountNonZero(img))

	fmt.Printf("White area = %.0f\n", whiteArea)
	return (whiteArea / size) * 100
}

func getCloudsPercentage(saveImage bool) (float64, error) {
	img, err := getRadarImage()
	if err != nil {
		return 0, err
	}
	defer img.Close()

	// Generate masks for different intensity levels
	maskLow, err := getDBzMask(img, RadarIntensityLow)
	if err != nil {
		return 0, err
	}
	defer maskLow.Close()

	maskMid, err := getDBzMask(img, RadarIntensityMid)
	if err != nil {
		return 0, err
	}
	defer maskMid.Close()

	maskHigh, err := getDBzMask(img, RadarIntensityHigh)
	if err != nil {
		return 0, err
	}
	defer maskHigh.Close()

	// Combine all masks
	combinedMask := gocv.NewMat()
	defer combinedMask.Close()
	gocv.Add(maskLow, maskMid, &combinedMask)
	gocv.Add(combinedMask, maskHigh, &combinedMask)

	cloudsArea := gocv.NewMat()
	defer cloudsArea.Close()
	gocv.BitwiseAndWithMask(img, img, &cloudsArea, combinedMask)

	// Crop the image
	roi := image.Rect(startX, startY, endX, endY)
	if startX < 0 || startY < 0 || endX > cloudsArea.Cols() || endY > cloudsArea.Rows() {
		return 0, fmt.Errorf("invalid cropping coordinates")
	}
	maskCropped := combinedMask.Region(roi)

	// Calculate the percentage of white pixels in the cropped mask
	cloudsPercent := getWhitePercentage(maskCropped)
	fmt.Printf("Clouds percentage: %.1f%%\n", cloudsPercent)

	// Draw rectangle and add text to the original image
	gocv.Rectangle(&img, roi, color.RGBA{0, 255, 0, 0}, 2)

	text := fmt.Sprintf("Clouds: %.1f%%", cloudsPercent)
	gocv.PutText(&img, text, image.Pt(10, 30), gocv.FontHersheyPlain, 1.5, color.RGBA{0, 255, 0, 0}, 2)

	tmpImagePath := "radar_tmp.jpg"
	if ok := gocv.IMWrite(tmpImagePath, img); !ok {
		fmt.Printf("Failed to save %s\n", tmpImagePath)
	}

	if saveImage {
		now := time.Now()
		filename := now.Format("radar_2006_01_02_15_04_05")
		radarImagePath := fmt.Sprintf("%s%s_radar.jpg", ImgPath, filename)
		segmentedImagePath := fmt.Sprintf("%s%s_segmented.jpg", ImgPath, filename)

		if ok := gocv.IMWrite(radarImagePath, img); !ok {
			fmt.Printf("Failed to save %s\n", radarImagePath)
		}
		if ok := gocv.IMWrite(segmentedImagePath, cloudsArea); !ok {
			fmt.Printf("Failed to save %s\n", segmentedImagePath)
		}
	}

	return cloudsPercent, nil
}
