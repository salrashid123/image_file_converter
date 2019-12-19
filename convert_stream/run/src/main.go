package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/genproto/googleapis/api/monitoredres"
	"gopkg.in/gographics/imagick.v2/imagick"

	//"contrib.go.opencensus.io/exporter/stackdriver/propagation"

	"contrib.go.opencensus.io/exporter/stackdriver"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	"github.com/gorilla/mux"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/trace"
)

var (
	bucketName = "YOUR_DEST_BUCKET-cdn"
	cdnURL     = "YOUR_CDN_URL"

	mBytes      = stats.Int64("BytesTransformed", "# bytesTransformed of called..", stats.UnitBytes)
	filetype, _ = tag.NewKey("filetype")
	countView   = &view.View{
		Name:        "mBytes/bytes",
		Measure:     mBytes,
		Description: "The number of bytes processed",
		Aggregation: ochttp.DefaultSizeDistribution,
		TagKeys:     []tag.Key{filetype},
	}
)

func transformHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		http.Error(w, "Method not allowed, use POST", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(0)
	if err != nil {
		log.Printf("Unable to parse Form")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	format := r.FormValue("format")
	fileName := r.FormValue("name") + "." + format

	file, _, err := r.FormFile("image")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	var buf bytes.Buffer
	io.Copy(&buf, file)
	img := buf.Bytes()
	buf.Reset()

	ctx := r.Context()

	_, requestSpan := trace.StartSpan(ctx, "start=request")

	// TODO: validate the filename, data here

	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer gcsClient.Close()

	dstBucket := gcsClient.Bucket(bucketName)
	gcsDstObject := dstBucket.Object(fileName)
	gcsDstWriter := gcsDstObject.NewWriter(ctx)

	mw := imagick.NewMagickWand()
	defer mw.Destroy()

	_, transformSpan := trace.StartSpan(ctx, "start=transform")
	err = mw.ReadImageBlob([]byte(img))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	mw.SetFormat(format)

	width := mw.GetImageWidth()
	height := mw.GetImageHeight()
	hWidth := uint(width / 2)
	hHeight := uint(height / 2)

	err = mw.ResizeImage(hWidth, hHeight, imagick.FILTER_LANCZOS, 1)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	transformSpan.End()

	attr := gcsDstWriter.ObjectAttrs
	attr.ContentType = "image/" + format

	n, err := gcsDstWriter.Write(mw.GetImageBlob())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer func(n int, ft string) {
		ctx, err := tag.New(context.Background(), tag.Insert(filetype, ft))
		if err != nil {
			log.Println(err)
		}
		stats.Record(ctx, mBytes.M(int64(n)))
	}(n, format)

	log.Printf("%d bytes are received.\n", n)

	err = gcsDstWriter.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	/*
		// Optionally set the ACL of the object to public read (for the CDN)
		acl := gcsDstObject.ACL()
		if err := acl.Set(ctx, storage.AllUsers, storage.RoleReader); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	*/

	requestSpan.End()

	if cdnURL != "" {
		w.Write([]byte(cdnURL + "/" + fileName))
	} else {
		w.Header().Set("Content-Type", "image/"+format)
		w.Write(mw.GetImageBlob())
	}
}

func main() {

	bucketName = os.Getenv("DEST_BUCKET_NAME")
	cdnURL = os.Getenv("CDN_URL")

	sd, err := stackdriver.NewExporter(stackdriver.Options{
		ProjectID: os.Getenv("PROJECT_NUMBER"),
		Resource: &monitoredres.MonitoredResource{
			// https://cloud.google.com/monitoring/custom-metrics/creating-metrics#which-resource
			Type: "generic_task",
			Labels: map[string]string{
				"project_id": os.Getenv("PROJECT_NUMBER"),
				"location":   "us-central1-a",
				"namespace":  "default",
				"job":        os.Getenv("K_SERVICE"),
				"task_id":    os.Getenv("K_REVISION"),
			},
		},
		DefaultMonitoringLabels: &stackdriver.Labels{},
	})
	if err != nil {
		log.Fatal(err)
	}

	trace.ApplyConfig(trace.Config{
		DefaultSampler: trace.AlwaysSample(),
	})
	trace.RegisterExporter(sd)

	if err := view.Register(countView); err != nil {
		log.Fatal(err)
	}

	view.SetReportingPeriod(60 * time.Second)
	view.RegisterExporter(sd)

	router := mux.NewRouter()

	router.Methods("POST").Path("/convert").HandlerFunc(transformHandler)

	log.Printf("Starting server")

	log.Fatal(http.ListenAndServe(":8080", &ochttp.Handler{
		Handler:          router,
		IsPublicEndpoint: false,
		GetStartOptions: func(r *http.Request) trace.StartOptions {
			startOptions := trace.StartOptions{}
			if r.UserAgent() == "GoogleHC/1.0" {
				startOptions.Sampler = trace.NeverSample()
			}
			return startOptions
		},
	}))

}
