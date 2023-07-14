package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/line/line-bot-sdk-go/linebot"
	"golang.org/x/exp/slog"
)

var (
	// DefaultHTTPGetAddress Default Address
	DefaultHTTPGetAddress = "https://checkip.amazonaws.com"

	// ErrNoIP No IP found in response
	ErrNoIP = errors.New("No IP in HTTP response")

	// ErrNon200Response non 200 status code in response
	ErrNon200Response = errors.New("Non 200 Response found")
)

func handler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	lcs, err := fetchParameterStore("LINE_CHANNEL_SECRET")
	if err != nil {
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, err
	}

	lcat, err := fetchParameterStore("LINE_CHANNEL_ACCESS_TOKEN")
	if err != nil {
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, err
	}

	slog.Info("hello world!", "name", "blue", "No", 5)
	log.Printf("MAKISHIMA_EVENT1: %s", request)
	log.Printf("MAKISHIMA_EVENT1.5: %s", request.Headers)
	log.Printf("MAKISHIMA_EVENT1.5: %s", request.Headers["x-line-signature"])
	// log.Printf("MAKISHIMA_EVENT1.5: %s", res)

	headers := http.Header{}
	headers.Add("X-Line-Signature", request.Headers["x-line-signature"])

	bot, err := linebot.New(lcs, lcat)
	if err != nil {
		log.Print(err)
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, err
		// return events.APIGatewayProxyResponse{StatusCode: http.StatusCreated}, err
	}
	log.Printf("MAKISHIMA_EVENT2: %s", bot)
	// log.Print(bot)

	// http.Requestを作成
	httpRequest, err := http.NewRequest(request.HTTPMethod, request.Path, strings.NewReader(request.Body))
	if err != nil {
		log.Print(err)
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, err
	}
	log.Printf("MAKISHIMA_EVENT3: %s", httpRequest)
	httpRequest.Header = headers
	log.Printf("MAKISHIMA_EVENT3: %s", httpRequest)

	// // ParseRequestにhttp.Requestを渡す
	botEvents, err := bot.ParseRequest(httpRequest)
	if err != nil {
		// log.Print(err)
		log.Printf("MAKISHIMA_EVENT4: %s", botEvents)
		log.Printf("MAKISHIMA_EVENT5: %s", err)
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, err
		// return events.APIGatewayProxyResponse{StatusCode: http.StatusCreated}, err
	}
	log.Printf("MAKISHIMA_EVENT6: %s", botEvents)

	for _, event := range botEvents {
		log.Printf("MAKISHIMA_EVENT7: %s", event)
		if event.Type == linebot.EventTypeMessage {
			switch message := event.Message.(type) {
			case *linebot.TextMessage:
				// 受信したテキストメッセージを取得
				text := message.Text

				log.Printf("MAKISHIMA_EVENT8: %s", text)
				slog.Info("message", "TEXT", text)

				replyText := doPost(text)
				log.Printf("MAKISHIMA_EVENT9: %s", replyText)
				slog.Info("message", "REPLY", replyText)

				if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage(replyText)).Do(); err != nil {
					log.Print(err)
					log.Printf("MAKISHIMA_EVENT10: %s", text)
				}
			}
		}
	}

	// 正常なレスポンスを返す
	return events.APIGatewayProxyResponse{StatusCode: http.StatusOK, Body: fmt.Sprintf("Hello, %v", string(""))}, nil
}

// メッセージを追加する関数
func addMessage(messages []linebot.SendingMessage, text string) []linebot.SendingMessage {
	// テキストメッセージを作成
	message := linebot.NewTextMessage(text)

	// メッセージを追加
	messages = append(messages, message)

	return messages
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func doPost(text string) string {
	url := "https://api.openai.com/v1/chat/completions"

	oak, err := fetchParameterStore("OPEN_API_KEY")
	if err != nil {
		// return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, err
	}

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
		log.Printf("GPT_EVENT1: %s", err)
		// log.Fatal(err)
	}

	// POSTリクエストを作成
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("GPT_EVENT2: %s", err)
		// log.Fatal(err)
	}

	// リクエストヘッダーの設定
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+oak)

	// HTTPクライアントを作成
	client := &http.Client{Timeout: 30 * time.Second}

	// リクエストを送信し、レスポンスを受け取る
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("GPT_EVENT3: %s", err)
		// log.Fatal(err)
	}
	// defer resp.Body.Close()

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			// panic(err)
			log.Printf("GPT_EVENT4: %s", err)
		}
	}(resp.Body)

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("GPT_EVENT5: %s", err)
		// panic(err)
	}

	var response OpenaiResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		log.Printf("GPT_EVENT6: %s", err)
		// return OpenaiResponse{}
	}

	// messages = append(messages, Message{
	// 	Role:    "assistant",
	// 	Content: response.Choices[0].Messages.Content,
	// })
	ans := response.Choices[0].Messages.Content
	return ans
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

// パラメータストアから設定値取得
func fetchParameterStore(param string) (string, error) {

	sess := session.Must(session.NewSession())
	svc := ssm.New(
		sess,
		aws.NewConfig().WithRegion("ap-northeast-1"),
	)

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
