package resourceacl

import "testing"

func TestCanonicalExportsPaths(t *testing.T) {
	for _, value := range []string{
		"local://7/exports/image.png",
		"minio://bucket/7/exports/image.webp",
		"cos://bucket/region/7/exports/image.jpg",
		"oss://bucket/exports/7/image.gif",
		"storage://backend-a/s3://bucket/prefix/7/exports/image.png",
	} {
		if !validPhysical(value, 7) {
			t.Errorf("rejected canonical image %q", value)
		}
	}
	for _, value := range []string{
		"local://8/exports/image.png",
		"local://7/document/image.png",
		"local://7/exports/private.pdf",
		"local://7/exports/../private.png",
		"local://7/exports/%2e%2e/private.png",
		"local://7//exports/image.png",
		"local://7/exports/image.png?token=secret",
		"local://7/exports/image.png#fragment",
		"local://7/exports/image.png\n",
		"local://7/exports/image\\other.png",
		"minio://12/7/exports/image.png", // native first numeric segment is 12
		"storage://../local://7/exports/image.png",
		"https://example.test/7/exports/image.png",
		"resource://abcdefghijklmnopqrstuv",
		"local://07/exports/image.png",
	} {
		if validPhysical(value, 7) {
			t.Errorf("accepted noncanonical image %q", value)
		}
	}
}

func TestHandleAndImmutableFingerprint(t *testing.T) {
	for _, reference := range []string{"resource://abcdefghijklmnopqrstuv", "resource://01234567890123456789_-"} {
		if _, ok := handleOf(reference); !ok {
			t.Fatal(reference)
		}
	}
	for _, reference := range []string{"resource://short", "resource://abcdefghijklmnopqrstuvw", "resource://abcdefghijklmnopqrstuv/x", " resource://abcdefghijklmnopqrstuv", "resource://abcdefghijklmnopqrstu%"} {
		if _, ok := handleOf(reference); ok {
			t.Fatal(reference)
		}
	}
	original := fingerprint(7, "resource-a", "local://7/exports/a.png")
	for _, candidate := range []string{
		fingerprint(8, "resource-a", "local://7/exports/a.png"),
		fingerprint(7, "resource-b", "local://7/exports/a.png"),
		fingerprint(7, "resource-a", "local://7/exports/b.png"),
	} {
		if candidate == original {
			t.Fatal("identity replacement retained approval fingerprint")
		}
	}
}
