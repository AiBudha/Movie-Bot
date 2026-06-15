package middleware

import (
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

var (
	userMessageCooldowns  = make(map[int64]time.Time)
	userCallbackCooldowns = make(map[int64]time.Time)
	cooldownMu            sync.RWMutex
)

// AntiSpam prevents users from making requests too frequently.
func AntiSpam(cooldown time.Duration, isAdmin func(int64) bool) func(bot *gotgbot.Bot, ctx *ext.Context) error {
	return func(bot *gotgbot.Bot, ctx *ext.Context) error {
		if ctx.EffectiveUser == nil {
			return ext.ContinueGroups
		}
		userId := ctx.EffectiveUser.Id
		
		if isAdmin != nil && isAdmin(userId) {
			return ext.ContinueGroups // Bypass rate-limiting for admins and continue processing
		}

		isCallback := ctx.CallbackQuery != nil

		cooldownMu.RLock()
		var lastSeen time.Time
		var exists bool
		if isCallback {
			lastSeen, exists = userCallbackCooldowns[userId]
		} else {
			lastSeen, exists = userMessageCooldowns[userId]
		}
		cooldownMu.RUnlock()

		if exists && time.Since(lastSeen) < cooldown {
			// Silently ignore or answer callback
			if isCallback {
				ctx.CallbackQuery.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{
					Text: "Slow down! Wait a few seconds.",
				})
			}
			return ext.EndGroups // Stop processing this update
		}

		cooldownMu.Lock()
		if isCallback {
			userCallbackCooldowns[userId] = time.Now()
		} else {
			userMessageCooldowns[userId] = time.Now()
		}
		cooldownMu.Unlock()

		return ext.ContinueGroups // Let subsequent handler groups process the update
	}
}

func init() {
	// Cleanup old entries every hour
	go func() {
		ticker := time.NewTicker(time.Hour)
		for range ticker.C {
			cooldownMu.Lock()
			for id, lastSeen := range userMessageCooldowns {
				if time.Since(lastSeen) > time.Hour {
					delete(userMessageCooldowns, id)
				}
			}
			for id, lastSeen := range userCallbackCooldowns {
				if time.Since(lastSeen) > time.Hour {
					delete(userCallbackCooldowns, id)
				}
			}
			cooldownMu.Unlock()
		}
	}()
}
