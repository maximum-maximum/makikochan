package main

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	"github.com/line/line-bot-sdk-go/linebot"
	"golang.org/x/exp/slog"
)

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenaiRequest struct {
	Model    string     `json:"model"`
	Messages []Message2 `json:"messages"`
}

type OpenaiResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int      `json:"created"`
	Choices []Choice `json:"choices"`
	Usages  Usage    `json:"usage"`
}

type Choice struct {
	Index        int     `json:"index"`
	Messages     Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Message2 struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func handler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	sess := session.Must(session.NewSession())
	svc := ssm.New(
		sess,
		aws.NewConfig().WithRegion("ap-northeast-1"),
	)

	lcs, err := fetchParameterStore("LINE_CHANNEL_SECRET", svc)
	if err != nil {
		slog.Error("GETTING_PARAMETER_FAILED", "PARAMETER_NAME", "LINE_CHANNEL_SECRET", "ERR", err)
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, err
	}

	lcat, err := fetchParameterStore("LINE_CHANNEL_ACCESS_TOKEN", svc)
	if err != nil {
		slog.Error("GETTING_PARAMETER_FAILED", "PARAMETER_NAME", "LINE_CHANNEL_ACCESS_TOKEN", "ERR", err)
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, err
	}

	oak, err := fetchParameterStore("OPEN_API_KEY", svc)
	if err != nil {
		slog.Error("GETTING_PARAMETER_FAILED", "PARAMETER_NAME", "OPEN_API_KEY", "ERR", err)
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, err
	}

	headers := http.Header{}
	headers.Add("X-Line-Signature", request.Headers["x-line-signature"])

	bot, err := linebot.New(lcs, lcat)
	if err != nil {
		slog.Error("CREATING_BOT_INSTANCE_FAILED", "ERR", err)
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, err
	}

	// http.Requestを作成
	httpRequest, err := http.NewRequest(request.HTTPMethod, request.Path, strings.NewReader(request.Body))
	if err != nil {
		slog.Error("CREATING_REQUEST_FAILED", "ERR", err)
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, err
	}

	httpRequest.Header = headers

	// ParseRequestにhttp.Requestを渡す
	botEvents, err := bot.ParseRequest(httpRequest)
	if err != nil {
		slog.Error("PARSING_REQUEST_FAILED", "ERR", err)
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, err
	}

	for _, event := range botEvents {
		if event.Type == linebot.EventTypeMessage {
			switch message := event.Message.(type) {
			case *linebot.TextMessage:
				// 受信したテキストメッセージを取得
				text := message.Text
				slog.Info("CLIENT_MESSAGE_TEXT", "TEXT", text)

				// テキストメッセージを元にChatGPTからの回答を取得
				replyText, err := getGptReply(text, oak, svc)
				if err != nil {
					slog.Error("GETTING_REPLY_FAILED", "ERR", err)
					return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, err
				}
				slog.Info("GPT_MESSAGE_TEXT", "TEXT", replyText)

				if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage(replyText)).Do(); err != nil {
					slog.Error("REPLY_FAILED", "ERR", err)
				}
			}
		}
	}

	// 正常なレスポンスを返す
	return events.APIGatewayProxyResponse{StatusCode: http.StatusOK}, nil
}

func getGptReply(text, oak string, svc ssmiface.SSMAPI) (str string, err error) {
	url := "https://api.openai.com/v1/chat/completions"

	// ChatRequestを作成
	chatRequest := ChatRequest{
		Model: "gpt-3.5-turbo",
		Messages: []Message{
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	// ChatRequestをJSONに変換
	jsonData, err := json.Marshal(chatRequest)
	if err != nil {
		slog.Error("MARSHALING_JSON_FAILED", "ERR", err)
		return "", err
	}

	// POSTリクエストを作成
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		slog.Error("CREATING_REQUEST_FAILED", "ERR", err)
		return "", err
	}

	// リクエストヘッダーの設定
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+oak)

	// HTTPクライアントを作成
	client := &http.Client{Timeout: 30 * time.Second}

	// リクエストを送信し、レスポンスを受け取る
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("SENDING_REQUEST_FAILED", "ERR", err)
		return "", err
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Error("CLOSING_BODY_FAILED", "ERR", err)
		}
	}(resp.Body)

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		slog.Error("READING_BODY_FAILED", "ERR", err)
		return "", err
	}

	var response OpenaiResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		slog.Error("UNMARSHALING_JSON_FAILED", "ERR", err)
		return "", err
	}

	ans := response.Choices[0].Messages.Content
	return ans, err
}

// パラメータストアから設定値取得
func fetchParameterStore(param string, svc ssmiface.SSMAPI) (string, error) {
	res, err := svc.GetParameter(&ssm.GetParameterInput{
		Name:           aws.String(param),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return "Fetch Error", err
	}

	value := *res.Parameter.Value
	return value, nil
}

func main() {
	lambda.Start(handler)
}
