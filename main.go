package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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
		ExposureTime            float64
		FNumber                 float64
		ISO                     int
		DateTimeOriginal        time.Time
		FocalLength             int
		FocalLengthIn35mmFormat int
		LensMake                string
		LensModel               string
	}
)

var (
	port         = envOrDefault("PORT", "8080")
	directusHost = envOrDefault("DIRECTUS_HOST", "https://content.carterjs.com")
	folderID     = envOrDefault("DIRECTUS_FOLDER_ID", "a0727005-2c49-47d3-a14c-c2d69e928854")
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
	"getAssetURL": func(photo Photo) string {
		return fmt.Sprintf("%s/assets/%s", directusHost, photo.ID)
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

		if photo.Meta.EXIF.FocalLengthIn35mmFormat != 0 {
			return fmt.Sprintf("%dmm (%dmm FFE)", photo.Meta.EXIF.FocalLength, photo.Meta.EXIF.FocalLengthIn35mmFormat)
		}

		return fmt.Sprintf("%dmm", photo.Meta.EXIF.FocalLength)
	},
	"displayExposure": func(photo Photo) string {
		if photo.Meta.EXIF.ExposureTime == 0 {
			return ""
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
		if photo.Meta.EXIF.ISO == 0 {
			return ""
		}

		return fmt.Sprintf("ISO %d", photo.Meta.EXIF.ISO)
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
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body struct {
		Data []Photo `json:"data"`
	}
	err = json.NewDecoder(resp.Body).Decode(&body)
	if err != nil {
		return nil, err
	}

	return body.Data, nil
}
