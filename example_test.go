package gotenberg_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	gotenberg "github.com/NicatorBa/GotenbergGo"
)

func ExampleNewClient() {
	client, err := gotenberg.NewClient("http://localhost:3000",
		gotenberg.WithBasicAuth("user", "password"),
	)
	if err != nil {
		log.Fatal(err)
	}

	_ = client
}

func ExampleChromium_ConvertFromURL() {
	client, err := gotenberg.NewClient("http://localhost:3000")
	if err != nil {
		log.Fatal(err)
	}

	_, err = client.Chromium().ConvertFromURL("https://example.com").
		PaperSize(gotenberg.PaperSizeA4).
		MarginTop("1cm").
		PrintBackground(true).
		ToFile(context.Background(), "example.pdf")
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleChromium_ConvertFromHTML() {
	client, err := gotenberg.NewClient("http://localhost:3000")
	if err != nil {
		log.Fatal(err)
	}

	res, err := client.Chromium().ConvertFromHTML().
		IndexFromPath("templates/report.html").
		FooterFromPath("templates/footer.html").
		FileFromPath("templates/style.css").
		Stages(gotenberg.StageStampText("DRAFT").Position(gotenberg.StampPositionCenter).Opacity(0.3)).
		ToBytes(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(res.ContentType, len(res.Content))
}

func ExampleLibreOffice_Convert() {
	client, err := gotenberg.NewClient("http://localhost:3000")
	if err != nil {
		log.Fatal(err)
	}

	// The documents are merged in the order they were added.
	_, err = client.LibreOffice().Convert().
		FileFromPath("cover.docx").
		FileFromPath("figures.xlsx").
		Stages(gotenberg.StageMerge(), gotenberg.StageRemediate(gotenberg.ProfilePDFA2b)).
		ToFile(context.Background(), "report.pdf")
	if err != nil {
		log.Fatal(err)
	}
}

func ExamplePDF_Pipe() {
	client, err := gotenberg.NewClient("http://localhost:3000")
	if err != nil {
		log.Fatal(err)
	}

	// Files used by a stage are uploaded too and referenced by filename.
	rr, err := client.PDF().Pipe().
		FileFromPath("in.pdf").
		FileFromPath("logo.png").
		Stages(
			gotenberg.StageStampImage("logo.png").Position(gotenberg.StampPositionTopRight).Width(80),
			gotenberg.StageEncrypt().UserPassword("secret").Permissions(gotenberg.Permissions{Print: true}),
		).
		ToReader(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = rr.Close() }()
}

func ExamplePDF_Info() {
	client, err := gotenberg.NewClient("http://localhost:3000")
	if err != nil {
		log.Fatal(err)
	}

	info, err := client.PDF().Info().
		FileFromReader("in.pdf", strings.NewReader("%PDF-1.7 ...")).
		Send(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(info["in.pdf"].Pages)
}

func ExamplePDF_Validate() {
	client, err := gotenberg.NewClient("http://localhost:3000")
	if err != nil {
		log.Fatal(err)
	}

	reports, err := client.PDF().Validate(gotenberg.ProfilePDFA2b).
		PairedWith(gotenberg.ProfilePDFUA1).
		FileFromPath("in.pdf").
		Send(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(reports["in.pdf"].Compliant)
}

func ExampleAPIError() {
	client, err := gotenberg.NewClient("http://localhost:3000")
	if err != nil {
		log.Fatal(err)
	}

	_, err = client.Chromium().ConvertFromURL("https://example.com").ToBytes(context.Background())

	var apiErr *gotenberg.APIError
	if errors.As(err, &apiErr) {
		fmt.Println(apiErr.StatusCode, apiErr.Message)
	}
}
