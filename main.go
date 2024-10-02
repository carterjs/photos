package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type (
	Photo struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
		Meta        Meta   `json:"metadata"`
	}
	Meta struct {
		IFD0 `json:"ifd0"`
		EXIF `json:"exif"`
	}
	IFD0 struct {
		Make  string
		Model string
	}
	EXIF struct {
		ExposureTime          float64
		FNumber               float64
		ISOSpeedRatings       int
		DateTimeOriginal      time.Time
		FocalLength           float64
		FocalLengthIn35mmFilm float64
		LensMake              string
		LensModel             string
	}
)

var (
	port          = envOrDefault("PORT", "8080")
	directusHost  = envOrDefault("DIRECTUS_HOST", "https://content.carterjs.com")
	folderID      = envOrDefault("DIRECTUS_FOLDER_ID", "360ad7fe-dbe0-4ffc-af2b-9347027dc0a8")
	directusToken = envOrDefault("DIRECTUS_TOKEN", "")
)

func envOrDefault(key, defaultValue string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultValue
}

var (
	//go:embed assets/*
	assetsFS embed.FS

	//go:embed templates/*
	templatesFS embed.FS
)

var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"getPreviewURL": func(photo Photo) string {
		url := fmt.Sprintf("%s/assets/%s?key=card", directusHost, photo.ID)

		if directusToken != "" {
			url += "&access_token=" + directusToken
		}

		return url
	},
	"getAssetURL": func(photo Photo) string {
		url := fmt.Sprintf("%s/assets/%s?key=web", directusHost, photo.ID)

		if directusToken != "" {
			url += "&access_token=" + directusToken
		}

		return url
	},
	"displayCamera": func(photo Photo) string {
		if photo.Meta.IFD0.Make == "" {
			return ""
		}
		return fmt.Sprintf("%s %s", photo.Meta.IFD0.Make, photo.Meta.IFD0.Model)
	},
	"displayLens": func(photo Photo) string {
		if photo.Meta.EXIF.LensMake == "" {
			return ""
		}

		name := fmt.Sprintf("%s %s", photo.Meta.EXIF.LensMake, photo.Meta.EXIF.LensModel)

		// Remove null bytes (only a problem with Viltrox lens so far...)
		name = strings.ReplaceAll(name, "\x00", "")

		return name
	},
	"displayFocalLength": func(photo Photo) string {
		if photo.Meta.EXIF.FocalLength == 0 {
			return ""
		}

		focalLength := strconv.FormatFloat(photo.Meta.EXIF.FocalLength, 'f', -1, 64)
		focalLengthIn35mmFormat := strconv.FormatFloat(photo.Meta.FocalLengthIn35mmFilm, 'f', -1, 64)

		if photo.Meta.EXIF.FocalLengthIn35mmFilm != 0 {
			return fmt.Sprintf("%smm (%smm FFE)", focalLength, focalLengthIn35mmFormat)
		}

		return fmt.Sprintf("%smm", focalLength)
	},
	"displayExposure": func(photo Photo) string {
		if photo.Meta.EXIF.ExposureTime == 0 {
			return ""
		}

		if photo.Meta.EXIF.ExposureTime >= 1 {
			return fmt.Sprintf("%d sec", int(photo.Meta.EXIF.ExposureTime))
		}

		return fmt.Sprintf("1/%d sec", int(1/photo.Meta.EXIF.ExposureTime))
	},
	"displayAperture": func(photo Photo) string {
		if photo.Meta.EXIF.FNumber == 0 {
			return ""
		}

		return fmt.Sprintf("f/%.1f", photo.Meta.EXIF.FNumber)
	},
	"displayISO": func(photo Photo) string {
		if photo.Meta.EXIF.ISOSpeedRatings == 0 {
			return ""
		}

		return fmt.Sprintf("ISO %d", photo.Meta.EXIF.ISOSpeedRatings)
	},
	"getCopyrightYear": func() string {
		return strconv.Itoa(time.Now().Year())
	},
	"displayTime": func(photo Photo) string {
		if photo.Meta.EXIF.DateTimeOriginal.IsZero() {
			return ""
		}

		return photo.Meta.EXIF.DateTimeOriginal.Format("January 2, 2006 3:04 PM")
	},
}).ParseFS(templatesFS, "templates/*.tmpl"))

func main() {
	http.Handle("/assets/", http.FileServer(http.FS(assetsFS)))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// static assets only
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if r.URL.Path != "/" {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		// home page
		photos, err := getPhotos()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		err = templates.ExecuteTemplate(w, "home", photos)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	})

	log.Printf("Starting server on port %s", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatalln(err)
	}
}

func getPhotos() ([]Photo, error) {
	url := fmt.Sprintf("%s/files?fields=id,title,description,metadata,width,height&filter[folder][_eq]=%s", directusHost, folderID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if directusToken != "" {
		req.Header.Set("Authorization", "Bearer "+directusToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var body struct {
		Data []Photo `json:"data"`
	}
	err = json.NewDecoder(resp.Body).Decode(&body)
	if err != nil {
		return nil, err
	}

	// sort by original date
	sort.Slice(body.Data, func(i, j int) bool {
		return body.Data[i].Meta.EXIF.DateTimeOriginal.After(body.Data[j].Meta.EXIF.DateTimeOriginal)
	})

	return body.Data, nil
}
