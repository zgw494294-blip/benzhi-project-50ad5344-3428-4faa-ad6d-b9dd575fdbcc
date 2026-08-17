package review

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPWorkflowAndStrictJSON(t *testing.T) {
	server := httptest.NewServer(NewHandler(NewStore()))
	defer server.Close()

	var created Review
	status := sendJSON(t, server.Client(), http.MethodPost, server.URL+"/reviews", `{"sku":"CP-APPLE-12","checks":["ingredients","barcode"]}`, &created)
	if status != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", status, http.StatusCreated)
	}
	if created.Status != StatusOpen || len(created.Checks) != 2 {
		t.Fatalf("created review = %+v", created)
	}

	emptyNotePayload := `{"passed":true,"note":""}`
	var updated Review
	status = sendJSON(t, server.Client(), http.MethodPost, server.URL+"/reviews/"+created.ID+"/checks/ingredients", emptyNotePayload, &updated)
	if status != http.StatusOK || updated.Checks[0].Verdict == nil || updated.Checks[0].Verdict.Note == nil {
		t.Fatalf("verdict response status=%d body=%+v", status, updated)
	}

	status = sendJSON(t, server.Client(), http.MethodPost, server.URL+"/reviews/"+created.ID+"/checks/barcode", `{"passed":true}`, &updated)
	if status != http.StatusOK {
		t.Fatalf("second verdict status = %d, want %d", status, http.StatusOK)
	}
	status = sendJSON(t, server.Client(), http.MethodPost, server.URL+"/reviews/"+created.ID+"/finalize", "", &updated)
	if status != http.StatusOK || updated.Status != StatusApproved {
		t.Fatalf("finalize status=%d review=%+v", status, updated)
	}
	status = sendJSON(t, server.Client(), http.MethodGet, server.URL+"/reviews/"+created.ID, "", &updated)
	if status != http.StatusOK || updated.Status != StatusApproved {
		t.Fatalf("get status=%d review=%+v", status, updated)
	}

	status = sendJSON(t, server.Client(), http.MethodPost, server.URL+"/reviews", `{"sku":"CP-OTHER","checks":["label"],"extra":true}`, &Review{})
	if status != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, want %d", status, http.StatusBadRequest)
	}
	status = sendJSON(t, server.Client(), http.MethodPost, server.URL+"/reviews", `{"sku":"CP-OTHER","checks":["label"]} {}`, &Review{})
	if status != http.StatusBadRequest {
		t.Fatalf("trailing JSON status = %d, want %d", status, http.StatusBadRequest)
	}
	status = sendJSON(t, server.Client(), http.MethodGet, server.URL+"/reviews/missing", "", &errorResponse{})
	if status != http.StatusNotFound {
		t.Fatalf("missing review status = %d, want %d", status, http.StatusNotFound)
	}
}

func TestHTTPRejectsMissingVerdictAndWrongMethodAsJSON(t *testing.T) {
	server := httptest.NewServer(NewHandler(NewStore()))
	defer server.Close()

	var created Review
	if status := sendJSON(t, server.Client(), http.MethodPost, server.URL+"/reviews", `{"sku":"CP-APPLE-12","checks":["ingredients"]}`, &created); status != http.StatusCreated {
		t.Fatalf("create status = %d", status)
	}
	status := sendJSON(t, server.Client(), http.MethodPost, server.URL+"/reviews/"+created.ID+"/checks/ingredients", `{}`, &errorResponse{})
	if status != http.StatusBadRequest {
		t.Fatalf("missing passed status = %d, want %d", status, http.StatusBadRequest)
	}
	status = sendJSON(t, server.Client(), http.MethodPut, server.URL+"/reviews", `{}`, &errorResponse{})
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method status = %d, want %d", status, http.StatusMethodNotAllowed)
	}
}

func sendJSON(t *testing.T, client *http.Client, method, endpoint, body string, destination any) int {
	t.Helper()
	request, err := http.NewRequest(method, endpoint, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if destination != nil {
		if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return response.StatusCode
}
