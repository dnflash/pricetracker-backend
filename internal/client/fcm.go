package client

import (
	"bytes"
	"encoding/json"
	"github.com/pkg/errors"
	"io"
	"net/http"
)

type FCMSendResponse struct {
	Success int             `json:"success"`
	Failure int             `json:"failure"`
	Results []FCMSendResult `json:"results"`
}

type FCMSendResult struct {
	Error *string `json:"error"`
}

type FCMSendRequest struct {
	Notification    FCMNotification `json:"notification"`
	Data            FCMData         `json:"data"`
	RegistrationIDs []string        `json:"registration_ids"`
}

type FCMNotification struct {
	Title       string `json:"title"`
	Body        string `json:"body"`
	ClickAction string `json:"click_action"`
	Sound       string `json:"sound"`
}

type FCMData struct {
	ItemID string `json:"item_id"`
}

func (c Client) FCMSendNotification(fcmReqBody FCMSendRequest) (FCMSendResponse, error) {
	reqBody, err := json.Marshal(fcmReqBody)
	if err != nil {
		return FCMSendResponse{}, errors.Wrapf(err, "FCMSendNotification: FCMSendRequest JSON marshalling error, req: %#v", fcmReqBody)
	}

	req, err := newRequest(http.MethodPost, "https://fcm.googleapis.com/fcm/send", bytes.NewReader(reqBody))
	if err != nil {
		return FCMSendResponse{}, errors.Wrapf(err, "FCMSendNotification: error creating HTTP request from body: %s", string(reqBody))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "key="+c.FCMKey)

	resp, err := c.Client.Do(req)
	if err != nil {
		return FCMSendResponse{}, errors.Wrapf(err, "FCMSendNotification: error doing request: %+v", req)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.Logger.Error("FCMSendNotification: error closing response body, req: %+v, err: %v", req, err)
		}
	}()

	fcmSendResp := FCMSendResponse{}
	bodyReader := http.MaxBytesReader(nil, resp.Body, 300000)
	respBody, err := io.ReadAll(bodyReader)
	if err != nil {
		return FCMSendResponse{}, errors.Wrapf(err,
			"FCMSendNotification: error reading FCMSendAPI response body, req: %+v, response body:\n%+v", req, string(respBody))
	}
	if err = json.NewDecoder(bytes.NewReader(respBody)).Decode(&fcmSendResp); err != nil {
		return FCMSendResponse{}, errors.Wrapf(err,
			"FCMSendNotification: error decoding FCMSendAPI response body, req: %+v, response body:\n%+v", req, string(respBody))
	}

	return fcmSendResp, nil
}
