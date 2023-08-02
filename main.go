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

// func main() {
// 	var cfg Config
// 	err := cleanenv.ReadConfig("config.yaml", &cfg)
// 	if err != nil {
// 		log.Fatalf("Cannot read config: %s", err)
// 	}

// 	loadDataFromFile(cfg.PollResults, &pollResults)
// 	loadDataFromFile(cfg.VotedUsers, &votedUsers)

// 	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
// 	if err != nil {
// 		log.Panic(err)
// 	}

// 	bot.Debug = true

// 	log.Printf("Authorized on account %s", bot.Self.UserName)

// 	// Создание клавиатуры
// 	var keyboard = tgbotapi.NewReplyKeyboard(
// 		tgbotapi.NewKeyboardButtonRow(
// 			tgbotapi.NewKeyboardButtonLocation("Поделиться местоположением"),
// 			tgbotapi.NewKeyboardButton("Пройти опрос"),
// 			tgbotapi.NewKeyboardButton("Показать результаты опроса"),
// 		),
// 	)

// 	u := tgbotapi.NewUpdate(0)
// 	u.Timeout = 60

// 	updates := bot.GetUpdatesChan(u)

// 	for update := range updates {
// 		// Обработка результатов опроса
// 		if update.PollAnswer != nil {
// 			userId := update.PollAnswer.User.ID
// 			if !votedUsers[userId] {
// 				for _, answerIndex := range update.PollAnswer.OptionIDs {
// 					mutex.Lock()
// 					pollResults[pollOptions[answerIndex]]++
// 					mutex.Unlock()
// 				}
// 				votedUsers[userId] = true

// 				saveDataToFile(cfg.PollResults, pollResults)
// 				saveDataToFile(cfg.VotedUsers, votedUsers)
// 			} else {
// 				msg := tgbotapi.NewMessage(update.PollAnswer.User.ID, "Вы уже проголосовали в этом опросе.")
// 				bot.Send(msg)
// 			}

// 			sendPollResults(bot, userId, keyboard)
// 		}

// 		if update.Message == nil {
// 			continue
// 		}

// 		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

// 		// Если сообщение от пользователя это геолокация, то вызывается функция getWeather
// 		if update.Message.Location != nil {
// 			weather, err := getWeather(update.Message.Location.Latitude, update.Message.Location.Longitude, cfg.WeatherKey)
// 			if err != nil {
// 				log.Printf("Failed to get weather: %s", err)
// 				continue
// 			}

// 			msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Температура в городе %s составляет %.2f℃", weather.Name, weather.Main.Temp-273.15))
// 			bot.Send(msg)
// 		} else if update.Message.Text == "Пройти опрос" {
// 			poll := tgbotapi.NewPoll(update.Message.Chat.ID, "Используешь ли ты GitHub Copilot и ChatGPT?", pollOptions...)
// 			poll.IsAnonymous = false
// 			_, err := bot.Send(poll)
// 			if err != nil {
// 				log.Printf("Failed to send poll: %s", err)
// 			}
// 		} else if update.Message.Text == "Показать результаты опроса" {
// 			sendPollResults(bot, update.Message.Chat.ID, keyboard)
// 		} else {
// 			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Пожалуйста, поделитесь своим местоположением или пройдите опрос.")
// 			msg.ReplyMarkup = keyboard
// 			bot.Send(msg)
// 		}
// 	}
// }

// main is the main function of the bot, which initializes the bot and processes updates.
func main() {
	var cfg Config
	err := cleanenv.ReadConfig("config.yaml", &cfg)
	if err != nil {
		log.Fatalf("Cannot read config: %s", err)
	}

	loadDataFromFile(cfg.PollResults, &pollResults)
	loadDataFromFile(cfg.VotedUsers, &votedUsers)

	bot, err := createBot(&cfg)
	if err != nil {
		log.Panic(err)
	}

	keyboard := createKeyboard()
	processUpdates(bot, &cfg, keyboard)
}

// createBot creates and initializes a new bot.
func createBot(cfg *Config) (*tgbotapi.BotAPI, error) {
	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, err
	}
	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)
	return bot, nil
}

// createKeyboard creates a keyboard for bot.
func createKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButtonLocation("Поделиться местоположением"),
			tgbotapi.NewKeyboardButton("Пройти опрос"),
			tgbotapi.NewKeyboardButton("Показать результаты опроса"),
		),
	)
}

// processUpdates handles new updates from bot.
func processUpdates(bot *tgbotapi.BotAPI, cfg *Config, keyboard tgbotapi.ReplyKeyboardMarkup) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)
	for update := range updates {
		processUpdate(bot, &update, cfg, keyboard)
	}
}

// processUpdate обрабатывает одно обновление от бота. Она анализирует тип обновления и вызывает соответствующие функции обработки.
// bot - это объект бота, который используется для отправки ответов пользователю.
// update - это обновление от бота, которое нужно обработать.
// cfg - это конфигурация приложения, которая может быть необходима для обработки обновлений (например, API-ключ для обращения к сервису погоды).
// keyboard - это клавиатура бота, которая добавляется к каждому отправленному ботом сообщению.
func processUpdate(bot *tgbotapi.BotAPI, update *tgbotapi.Update, cfg *Config, keyboard tgbotapi.ReplyKeyboardMarkup) {
	// Обработка результатов опроса
	if update.PollAnswer != nil {
		handlePollAnswer(bot, update, cfg, keyboard)
	}

	if update.Message == nil {
		return
	}

	log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

	// Если сообщение от пользователя это геолокация, то вызывается функция getWeather
	if update.Message.Location != nil {
		handleLocationMessage(bot, update, cfg)
	} else if update.Message.Text == "Пройти опрос" {
		handlePollRequest(bot, update)
	} else if update.Message.Text == "Показать результаты опроса" {
		sendPollResults(bot, update.Message.Chat.ID, keyboard)
	} else {
		handleUnknownMessage(bot, update, keyboard)
	}
}

func handlePollAnswer(bot *tgbotapi.BotAPI, update *tgbotapi.Update, cfg *Config, keyboard tgbotapi.ReplyKeyboardMarkup) {
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
		sendPollResults(bot, userId, keyboard)
	} else {
		msg := tgbotapi.NewMessage(update.PollAnswer.User.ID, "Вы уже проголосовали в этом опросе.")
		bot.Send(msg)
	}
}

func handleLocationMessage(bot *tgbotapi.BotAPI, update *tgbotapi.Update, cfg *Config) {
	weather, err := getWeather(update.Message.Location.Latitude, update.Message.Location.Longitude, cfg.WeatherKey)
	if err != nil {
		log.Printf("Failed to get weather: %s", err)
		return
	}

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Температура в городе %s составляет %.2f℃", weather.Name, weather.Main.Temp-273.15))
	bot.Send(msg)
}

func handlePollRequest(bot *tgbotapi.BotAPI, update *tgbotapi.Update) {
	poll := tgbotapi.NewPoll(update.Message.Chat.ID, "Используешь ли ты GitHub Copilot и ChatGPT?", pollOptions...)
	poll.IsAnonymous = false
	_, err := bot.Send(poll)
	if err != nil {
		log.Printf("Failed to send poll: %s", err)
	}
}

func handleUnknownMessage(bot *tgbotapi.BotAPI, update *tgbotapi.Update, keyboard tgbotapi.ReplyKeyboardMarkup) {
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Пожалуйста, поделитесь своим местоположением или пройдите опрос.")
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
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

// sendPollResults отправляет результаты опроса в чат с указанным ID.
// Эта функция сначала блокирует доступ к глобальным данным опроса с помощью мьютекса,
// чтобы предотвратить гонки данных, затем собирает результаты опроса в строку и отправляет ее в чат.
//
// Параметры:
// - bot: объект BotAPI, который используется для отправки сообщений.
// - chatID: ID чата, в который отправляется сообщение.
// - keyboard: объект ReplyKeyboardMarkup, который используется в качестве клавиатуры для сообщения.
//
// Сообщение с результатами опроса состоит из списка вариантов ответа с соответствующим количеством голосов.
// Если при отправке сообщения возникает ошибка, информация об ошибке выводится в лог.
//
// Пример использования:
//
// keyboard := tgbotapi.NewReplyKeyboard(tgbotapi.NewKeyboardButtonRow(
//
//	tgbotapi.NewKeyboardButton("Показать результаты опроса"),
//
// ))
// sendPollResults(bot, update.Message.Chat.ID, keyboard)
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

// saveDataToFile сериализует предоставленные данные в формат JSON и записывает их в файл с указанным именем.
// Если происходит ошибка при сериализации данных или записи в файл, информация об ошибке выводится в лог.
//
// Параметры:
// - filename: Имя файла, в который следует записать данные.
// - data: Данные, которые следует записать в файл. Может быть любым типом данных, который можно сериализовать в JSON.
//
// Пример использования:
//
// saveDataToFile("data.json", myData)
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

// loadDataFromFile читает данные из файла с указанным именем и десериализует их из формата JSON.
// Если файл не существует, функция создает новый файл с пустым JSON-объектом ("{}").
// Если происходит ошибка при чтении из файла, создании нового файла или десериализации данных, информация об ошибке выводится в лог.
//
// Параметры:
// - filename: Имя файла, из которого следует прочитать данные.
// - data: Указатель на переменную, в которую следует десериализовать данные. Должен быть указателем на переменную подходящего типа.
//
// Пример использования:
//
// loadDataFromFile("data.json", &myData)
func loadDataFromFile(filename string, data interface{}) {
	// Check if file exists, create it if it doesn't
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		file, err := os.Create(filename)
		if err != nil {
			log.Printf("Failed to create file: %s", err)
			return
		}

		// добавляем в файл пустую json-структуру
		_, err = file.WriteString("{}")
		if err != nil {
			log.Printf("Failed to write to file: %s", err)
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
