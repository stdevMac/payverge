package server

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stdevMac/shares/internal/database"
	"github.com/stdevMac/shares/internal/metrics"
	"github.com/stdevMac/shares/internal/structs"
)

var bot *tgbotapi.BotAPI

func InitializeTelegramBot(telegramToken string) *tgbotapi.BotAPI {
	var err error
	bot, err = tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		log.Panic(err)
	}
	return bot
}

func WebhookHandler(c *gin.Context) {
	var update tgbotapi.Update
	if err := c.ShouldBindJSON(&update); err != nil {
		metrics.TelegramCommands.WithLabelValues("webhook", "error").Inc()
		log.Println("Failed to parse update:", err)
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	// Handle commands
	if update.Message != nil && update.Message.IsCommand() {
		metrics.TelegramCommands.WithLabelValues("webhook", "success").Inc()
		handleCommand(update.Message)
	}

	c.JSON(200, gin.H{"status": "ok"})
}

// handleCommand routes Telegram commands to their corresponding logic
func handleCommand(message *tgbotapi.Message) {
	startTime := time.Now()
	chatID := message.Chat.ID
	command := strings.ToLower(message.Command())
	args := message.CommandArguments()

	defer func() {
		// Record response time
		duration := time.Since(startTime).Seconds()
		metrics.TelegramResponseTime.WithLabelValues(command).Observe(duration)
	}()

	metrics.TelegramCommands.WithLabelValues(command, "received").Inc()

	switch command {
	case "start":
		handleStart(chatID, args)
	case "help":
		handleHelp(chatID)
	case "status":
		handleStatus(chatID)
	case "account":
		handleAccount(chatID)
	default:
		handleUnknown(chatID)
		metrics.TelegramCommands.WithLabelValues("unknown", "error").Inc()
		return
	}
}

// handleHelp handles the /help command
func handleHelp(chatID int64) {
	message := "🤖 *Available Commands*\n\n" +
		"*Basic Commands:*\n" +
		"/start - Connect your account\n" +
		"/help - Show this help message\n" +
		"/account - View your account information\n" +
		"/status - Get your account status\n\n" +
		"_More commands will be available based on your app's features._"

	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = "Markdown"
	if _, err := bot.Send(msg); err != nil {
		log.Println("Error sending help message:", err)
	}
}

// handleStatus handles the /status command
func handleStatus(chatID int64) {
	// Get user by chat ID
	user, err := getUserByChatID(chatID)
	if err != nil {
		message := "❌ *Account Error*\n\n" +
			"Unable to retrieve your account information.\n\n" +
			"🔍 *Possible Solutions*\n" +
			"• Ensure your account is connected\n" +
			"• Try reconnecting through the app\n" +
			"• Contact support if the issue persists\n\n" +
			"Need help? Contact support@yourapp.io"
		keyboard := getCommandKeyboard("error", structs.User{}, "", "")
		sendMessage(chatID, message, keyboard)
		return
	}

	message := fmt.Sprintf("📊 *Account Status*\n\n"+
		"👤 Account: Connected\n"+
		"🔑 Address: %s\n\n"+
		"_Your account is active and ready to use._",
		user.Address[:10]+"...")

	keyboard := getCommandKeyboard("status", user, "", "")
	sendMessage(chatID, message, keyboard)
}

// handleStart handles the /start command
func handleStart(chatID int64, args string) {
	metrics.TelegramCommands.WithLabelValues("start", "processing").Inc()
	message := "🚀 *Welcome to Your App Bot*\n\n"

	// Validate arguments (address or token)
	if args == "" {
		message += "❌ It seems you're trying to connect without a token. Please use the unique link provided to you.\n\n"
		message += "🔍 *Getting Started*\n"
		message += "1. Visit your app's website\n"
		message += "2. Connect your wallet\n"
		message += "3. Use the Telegram connect button\n\n"
		message += "Need help? Contact support@yourapp.io"

		keyboard := getCommandKeyboard("start", structs.User{}, "", "")
		sendMessage(chatID, message, keyboard)
		return
	}

	// Clean up and validate the address/token
	address := strings.TrimSpace(strings.ToLower(args))
	if !strings.HasPrefix(address, "0x") {
		message += "❌ *Invalid Token*\n\n"
		message += "The provided token is not valid. Please ensure you're using a valid address.\n\n"
		message += "🔍 *Need Help?*\n"
		message += "• Make sure you're using the link from the app\n"
		message += "• Contact support@yourapp.io for assistance"

		keyboard := getCommandKeyboard("start", structs.User{}, "", "")
		sendMessage(chatID, message, keyboard)
		return
	}

	// Fetch the user from the database
	user, err := database.GetUserByAddress(address)
	if err != nil {
		log.Println("Error retrieving user by address:", err)
		message += "❌ *Account Not Found*\n\n"
		message += "We couldn't find an account associated with this token.\n\n"
		message += "🔍 *Possible Solutions*\n"
		message += "• Verify you're using the correct link\n"
		message += "• Ensure your account is connected on the website\n"
		message += "• Try reconnecting through the app\n\n"
		message += "Need help? Contact support@yourapp.io"

		keyboard := getCommandKeyboard("start", structs.User{}, "", "")
		sendMessage(chatID, message, keyboard)
		return
	}

	// Update the user's Telegram Chat ID if not already set
	if user.TGChatID == 0 {
		user.TGChatID = chatID
		if err = database.UpdateUser(user); err != nil {
			metrics.TelegramCommands.WithLabelValues("start", "error").Inc()
			log.Println("Error updating user's Telegram Chat ID:", err)
			message += "❌ *Connection Error*\n\n"
			message += "An error occurred while linking your account. Please try again later.\n\n"
			message += "If the problem persists, contact support@yourapp.io"

			keyboard := getCommandKeyboard("start", structs.User{}, "", "")
			sendMessage(chatID, message, keyboard)
			return
		}
		message = "🎉 *Successfully Connected!*\n\n"
		// Update active users count
		updateTelegramActiveUsers()
		metrics.TelegramCommands.WithLabelValues("start", "success").Inc()
	} else if user.TGChatID != chatID {
		message = "⚠️ *Account Already Linked*\n\n"
		message += "This account is already connected to another Telegram user.\n"
		message += "For security reasons, each account can only be linked to one Telegram user.\n\n"
		message += "If this is unexpected, please contact support@yourapp.io"

		keyboard := getCommandKeyboard("start", structs.User{}, "", "")
		sendMessage(chatID, message, keyboard)
		return
	} else {
		message = "👋 *Welcome Back!*\n\n"
	}

	// Add command instructions
	message += "🎯 *Available Commands*\n"
	message += "• /status - Account status\n"
	message += "• /account - Account information\n"
	message += "• /help - See all commands\n\n"
	message += "_Ready to get started!_"

	keyboard := getCommandKeyboard("start", user, "", "")
	sendMessage(chatID, message, keyboard)
}

// handleUnknown handles unknown commands
func handleUnknown(chatID int64) {
	message := "🤔 *Command Not Recognized*\n\n"
	message += "I didn't understand that command.\n\n"
	message += "📚 Use /help to see all available commands\n"
	message += "❓ Need help? Contact support@yourapp.io"

	keyboard := getCommandKeyboard("unknown", structs.User{}, "", "")
	sendMessage(chatID, message, keyboard)
}

// getUserByChatID is a helper function to get user by Telegram chat ID
func getUserByChatID(chatID int64) (structs.User, error) {
	users, err := database.GetAllUsers()
	if err != nil {
		return structs.User{}, err
	}

	for _, user := range users {
		if user.TGChatID == chatID {
			return user, nil
		}
	}

	return structs.User{}, fmt.Errorf("user not found")
}

// sendMessage is a helper function to send messages to Telegram users
func sendMessage(chatID int64, text string, keyboard *tgbotapi.InlineKeyboardMarkup) {
	metrics.TelegramMessageLength.Observe(float64(len(text)))

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	if keyboard != nil {
		msg.ReplyMarkup = keyboard
	}
	if _, err := bot.Send(msg); err != nil {
		metrics.TelegramCommands.WithLabelValues("send_message", "error").Inc()
		log.Println("Error sending message:", err)
		return
	}
	metrics.TelegramCommands.WithLabelValues("send_message", "success").Inc()
}

// getCommandKeyboard creates inline keyboard with relevant buttons based on command context
func getCommandKeyboard(command string, user structs.User, entityID string, secondaryID string) *tgbotapi.InlineKeyboardMarkup {
	var buttons [][]tgbotapi.InlineKeyboardButton

	// Add command-specific buttons
	switch command {
	case "status", "account":
		dashboardButton := tgbotapi.NewInlineKeyboardButtonURL(
			"View Dashboard",
			"https://yourapp.io/dashboard",
		)
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{dashboardButton})

	case "error", "start":
		connectButton := tgbotapi.NewInlineKeyboardButtonURL(
			"Open App",
			"https://yourapp.io/",
		)
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{connectButton})
	}

	// Add referral button if user has a referral code (customize based on your app's needs)
	if user.ReferralCode != "" {
		referralButton := tgbotapi.NewInlineKeyboardButtonURL(
			"Share Referral Link",
			fmt.Sprintf("https://yourapp.io/referral/%s", user.ReferralCode),
		)
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{referralButton})
	}

	if len(buttons) == 0 {
		return nil
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)
	return &keyboard
}

// handleAccount shows user's account information
func handleAccount(chatID int64) {
	user, err := getUserByChatID(chatID)
	if err != nil {
		message := "❌ *Account Error*\n\n" +
			"Unable to retrieve your account information.\n\n" +
			"🔍 *Possible Solutions*\n" +
			"• Verify your account connection\n" +
			"• Try reconnecting through the app\n" +
			"• Contact support if the issue persists\n\n" +
			"Need help? Contact support@yourapp.io"
		keyboard := getCommandKeyboard("error", structs.User{}, "", "")
		sendMessage(chatID, message, keyboard)
		return
	}

	var message strings.Builder
	message.WriteString("👤 *Account Information*\n\n")

	// Account Address
	message.WriteString("🔑 *Account Address*\n")
	message.WriteString(fmt.Sprintf("`%s`\n\n", user.Address))

	// Account Status
	message.WriteString("📊 *Account Status*\n")
	message.WriteString("Status: Active\n")
	message.WriteString("Connected: Yes\n\n")

	// Additional Info (can be customized based on your app's needs)
	message.WriteString("📝 *Additional Information*\n")
	message.WriteString("_Account details and features will be displayed here based on your app's functionality._\n")

	keyboard := getCommandKeyboard("account", user, "", "")
	sendMessage(chatID, message.String(), keyboard)
}

// Add this helper function to update active users count
func updateTelegramActiveUsers() {
	users, err := database.GetAllUsers()
	if err != nil {
		log.Printf("Error getting users for metrics: %v", err)
		return
	}

	activeCount := 0
	for _, user := range users {
		if user.TGChatID != 0 {
			activeCount++
		}
	}

	metrics.TelegramActiveUsers.Set(float64(activeCount))
}
