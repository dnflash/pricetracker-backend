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
		return FCMSendResponse{}, errors.Wrapf(err, "FCMSendNotification: FCMSendRequest JSON marshalling error, req: %+v", fcmReqBody)
	}

	req, err := newRequest(http.MethodPost, "https://fcm.googleapis.com/fcm/send", bytes.NewReader(reqBody))
	if err != nil {
		return FCMSendResponse{}, errors.Wrapf(err, "FCMSendNotification: error creating HTTP request from body: %s", reqBody)
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
	respBody, err := io.ReadAll(http.MaxBytesReader(nil, resp.Body, 300000))
	if err != nil {
		return fcmSendResp, errors.Wrapf(err,
			"FCMSendNotification: error reading FCMSendAPI response body, req: %+v, response body: %s", req, respBody)
	}
	err = json.Unmarshal(respBody, &fcmSendResp)
	return fcmSendResp, errors.Wrapf(err,
		"FCMSendNotification: error unmarshalling FCMSendAPI response body, req: %+v, response body: %s", req, respBody)
}
