package structs

func (u *User) AddReferee(referee string) {
	if u.RefereePoints == nil {
		u.RefereePoints = make(map[string]int64)
	}
	u.RefereePoints[referee] = 0
}

func (u *User) AddRefereePoints(referrer string, points int64) {
	if u.RefereePoints == nil {
		u.RefereePoints = make(map[string]int64)
	}
	u.RefereePoints[referrer] += int64(points / 4) // 25% of the points
}

// AddNotification adds a new notification to the user
func (u *User) AddNotification(notification Notification) {
	u.Notifications = append(u.Notifications, notification)
}
