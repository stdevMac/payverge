package structs

// AddNotification adds a new notification to the user
func (u *User) AddNotification(notification Notification) {
	u.Notifications = append(u.Notifications, notification)
}
