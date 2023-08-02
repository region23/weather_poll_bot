package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	BotToken    string `yaml:"bot_token" env-required:"true"`
	WeatherKey  string `yaml:"weather_key" env-required:"true"`
	PollResults string `yaml:"poll_results" env-required:"true"`
	VotedUsers  string `yaml:"voted_users" env-required:"true"`
}

// Структура для парсинга JSON ответа от OpenWeatherMap API
type WeatherResponse struct {
	Main struct {
		Temp float64 `json:"temp"`
	} `json:"main"`
	Name string `json:"name"`
}

var pollResults = make(map[string]int)
var votedUsers = make(map[int64]bool)
var mutex = &sync.Mutex{}

var pollOptions = []string{
	"Использую в собственных проектах",
	"Уже использую в собственных проектах и на работе",
	"Не использую, но хотел бы использовать",
	"Мне это не интересно",
	"Не вижу в этом профита",
}

func main() {
	var cfg Config
	err := cleanenv.ReadConfig("config.yaml", &cfg)
	if err != nil {
		log.Fatalf("Cannot read config: %s", err)
	}

	loadDataFromFile(cfg.PollResults, &pollResults)
	loadDataFromFile(cfg.VotedUsers, &votedUsers)

	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Создание клавиатуры
	var keyboard = tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButtonLocation("Поделиться местоположением"),
			tgbotapi.NewKeyboardButton("Пройти опрос"),
			tgbotapi.NewKeyboardButton("Показать результаты опроса"),
		),
	)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		// Обработка результатов опроса
		if update.PollAnswer != nil {
			userId := update.PollAnswer.User.ID
			if !votedUsers[userId] {
				for _, answerIndex := range update.PollAnswer.OptionIDs {
					mutex.Lock()
					pollResults[pollOptions[answerIndex]]++
					mutex.Unlock()
				}
				votedUsers[userId] = true

				saveDataToFile(cfg.PollResults, pollResults)
				saveDataToFile(cfg.VotedUsers, votedUsers)
			} else {
				msg := tgbotapi.NewMessage(update.PollAnswer.User.ID, "Вы уже проголосовали в этом опросе.")
				bot.Send(msg)
			}

			sendPollResults(bot, userId, keyboard)
		}

		if update.Message == nil {
			continue
		}

		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

		// Если сообщение от пользователя это геолокация, то вызывается функция getWeather
		if update.Message.Location != nil {
			weather, err := getWeather(update.Message.Location.Latitude, update.Message.Location.Longitude, cfg.WeatherKey)
			if err != nil {
				log.Printf("Failed to get weather: %s", err)
				continue
			}

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Температура в городе %s составляет %.2f℃", weather.Name, weather.Main.Temp-273.15))
			bot.Send(msg)
		} else if update.Message.Text == "Пройти опрос" {
			poll := tgbotapi.NewPoll(update.Message.Chat.ID, "Используешь ли ты GitHub Copilot и ChatGPT?", pollOptions...)
			poll.IsAnonymous = false
			_, err := bot.Send(poll)
			if err != nil {
				log.Printf("Failed to send poll: %s", err)
			}
		} else if update.Message.Text == "Показать результаты опроса" {
			sendPollResults(bot, update.Message.Chat.ID, keyboard)
		} else {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Пожалуйста, поделитесь своим местоположением или пройдите опрос.")
			msg.ReplyMarkup = keyboard
			bot.Send(msg)
		}
	}
}

// Функция, которая делает запрос к OpenWeatherMap API и возвращает информацию о погоде
func getWeather(lat, lon float64, key string) (*WeatherResponse, error) {
	url := fmt.Sprintf("http://api.openweathermap.org/data/2.5/weather?lat=%.2f&lon=%.2f&appid=%s", lat, lon, key)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var weather WeatherResponse

	if err := json.Unmarshal(data, &weather); err != nil {
		return nil, err
	}

	return &weather, nil
}

func sendPollResults(bot *tgbotapi.BotAPI, chatID int64, keyboard tgbotapi.ReplyKeyboardMarkup) {
	mutex.Lock()
	defer mutex.Unlock()

	// Создание сообщения с результатами опроса
	var message strings.Builder
	message.WriteString("Результаты опроса:\n\n")

	for option, votes := range pollResults {
		message.WriteString(fmt.Sprintf("%s: %d\n", option, votes))
	}

	msg := tgbotapi.NewMessage(chatID, message.String())
	msg.ReplyMarkup = keyboard
	_, err := bot.Send(msg)
	if err != nil {
		log.Printf("Failed to send message: %s", err)
	}
}

func saveDataToFile(filename string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("Failed to serialize data: %s", err)
		return
	}

	err = os.WriteFile(filename, jsonData, 0644)
	if err != nil {
		log.Printf("Failed to write to file: %s", err)
	}
}

func loadDataFromFile(filename string, data interface{}) {
	// Check if file exists, create it if it doesn't
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		file, err := os.Create(filename)
		if err != nil {
			log.Printf("Failed to create file: %s", err)
			return
		}
		defer file.Close()
	}

	jsonData, err := os.ReadFile(filename)
	if err != nil {
		log.Printf("Failed to read from file: %s", err)
		return
	}

	err = json.Unmarshal(jsonData, data)
	if err != nil {
		log.Printf("Failed to deserialize data: %s", err)
	}
}
