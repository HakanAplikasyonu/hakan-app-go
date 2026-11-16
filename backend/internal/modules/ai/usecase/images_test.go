package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/redact"
)

type wordsOCR []redact.Word

func (w wordsOCR) Words(context.Context, []byte) ([]redact.Word, error) { return w, nil }

func receiptPNG(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 200, 60))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

func imageCall(t *testing.T) Call {
	return Call{OrganizationID: 1, Purpose: "receipt_ocr", Messages: []provider.Message{{
		Role: provider.RoleUser, Content: []provider.Block{
			provider.ImageBlock("image/png", receiptPNG(t)),
			provider.TextBlock("Fişi oku."),
		},
	}}}
}

// Maskeleme açıkken görsel, OCR motoru yoksa hiç gönderilmez (#169).
func TestImageCallRefusedWithoutOCR(t *testing.T) {
	fake := &provider.Fake{Responses: []provider.Response{textResp("ok", "m")}}
	svc := New(&Backend{Provider: fake, Model: "m"}, nil, nil, Options{Masking: true})
	if _, err := svc.Complete(context.Background(), imageCall(t)); !errors.Is(err, ErrImageRedactionUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if len(fake.Requests) != 0 {
		t.Fatal("image reached the provider")
	}
	if got := svc.Privacy().ImageRedaction; got != "unavailable" {
		t.Fatalf("privacy image_redaction = %q", got)
	}
	// Text-only calls are not affected.
	if _, err := svc.Complete(context.Background(), Call{OrganizationID: 1, Messages: userMsg("selam")}); err != nil {
		t.Fatalf("text call: %v", err)
	}
}

// Görsel karartılarak ve belirteç notuyla gider; yanıttaki belirteç sunucuda
// geri açılır.
func TestImageIsRedactedBeforeSending(t *testing.T) {
	fake := &provider.Fake{Responses: []provider.Response{textResp(`{"vendor_tax_number":"[VKN_1]"}`, "m")}}
	ocr := wordsOCR{
		{Text: "VKN", Line: 1, Box: image.Rect(5, 5, 40, 20)},
		{Text: "1234567890", Line: 1, Box: image.Rect(45, 5, 150, 20)},
	}
	svc := New(&Backend{Provider: fake, Model: "m"}, nil, nil, Options{Masking: true, ImageRedactor: redact.New(ocr)})
	call := imageCall(t)
	res, err := svc.Complete(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Message.Text(); got != `{"vendor_tax_number":"1234567890"}` {
		t.Fatalf("answer = %q", got)
	}
	sent := fake.Requests[0].Messages[0].Content
	if len(sent) != 3 || sent[0].Type != provider.BlockImage || !strings.Contains(sent[1].Text, "[VKN_1]") {
		t.Fatalf("sent blocks = %+v", sent)
	}
	if sent[0].Data == call.Messages[0].Content[0].Data {
		t.Fatal("original image was sent")
	}
	raw, _ := base64.StdEncoding.DecodeString(sent[0].Data)
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, _ := img.At(100, 12).RGBA(); r > 0x2000 {
		t.Fatal("VKN region is not black")
	}
	if r, _, _, _ := img.At(20, 12).RGBA(); r < 0xf000 {
		t.Fatal("label was blacked out")
	}
	if svc.Privacy().ImageRedaction != RedactionRequired {
		t.Fatalf("privacy = %+v", svc.Privacy())
	}
}

func TestImageSentAsIsWhenRedactionOffOrMaskingOff(t *testing.T) {
	for _, opts := range []Options{{Masking: true, ImageRedaction: RedactionOff}, {Masking: false}} {
		fake := &provider.Fake{Responses: []provider.Response{textResp("ok", "m")}}
		svc := New(&Backend{Provider: fake, Model: "m"}, nil, nil, opts)
		call := imageCall(t)
		if _, err := svc.Complete(context.Background(), call); err != nil {
			t.Fatalf("%+v: %v", opts, err)
		}
		if fake.Requests[0].Messages[0].Content[0].Data != call.Messages[0].Content[0].Data {
			t.Fatalf("%+v: image changed", opts)
		}
		if svc.Privacy().ImageRedaction != RedactionOff {
			t.Fatalf("%+v: privacy = %q", opts, svc.Privacy().ImageRedaction)
		}
	}
}
