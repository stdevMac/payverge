# Reservation Reminder Service Documentation

## Overview

The Reservation Reminder Service is a background job that automatically sends reminder emails to customers with upcoming reservations. It checks business settings to determine when reminders should be sent and processes reservations accordingly.

## Architecture

### Components

1. **ReservationReminderService** (`internal/services/reservation_reminder.go`)
   - Core service that processes reservations and sends emails
   - Checks business settings for reminder preferences
   - Sends confirmation and reminder emails

2. **ReservationScheduler** (`internal/jobs/reservation_scheduler.go`)
   - Cron-based scheduler that runs the service periodically
   - Runs every hour to check for upcoming reservations
   - Can be triggered manually for testing

3. **Database Model** (`internal/database/models.go`)
   - `TableReservation.ReminderSent` field tracks if reminder was sent
   - Prevents duplicate reminder emails

## Setup Instructions

### 1. Install Required Dependencies

Add the cron library to your `go.mod`:

```bash
cd backend
go get github.com/robfig/cron/v3
```

### 2. Run Database Migration

Add the `reminder_sent` column to the `table_reservations` table:

```sql
ALTER TABLE table_reservations 
ADD COLUMN reminder_sent BOOLEAN DEFAULT FALSE;

-- Add index for better query performance
CREATE INDEX idx_reservations_reminder 
ON table_reservations(business_id, reservation_time, reminder_sent, status);
```

### 3. Initialize Scheduler in Main Application

Add to your `main.go` or application startup:

```go
package main

import (
    "github.com/stdevmac/payverge/backend/internal/jobs"
    "log"
    "os"
    "os/signal"
    "syscall"
)

func main() {
    // ... your existing setup ...

    // Initialize reservation scheduler
    scheduler := jobs.NewReservationScheduler()
    if err := scheduler.Start(); err != nil {
        log.Fatalf("Failed to start reservation scheduler: %v", err)
    }

    // Graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    
    go func() {
        <-sigChan
        log.Println("Shutting down...")
        scheduler.Stop()
        os.Exit(0)
    }()

    // ... rest of your application ...
}
```

## How It Works

### 1. Reminder Processing Flow

```
Every Hour:
  ├─ Get all businesses with reservations enabled
  ├─ For each business:
  │   ├─ Check if SendReminderEmail is enabled
  │   ├─ Get ReminderHoursBefore setting (default: 24 hours)
  │   ├─ Calculate target time window (±30 minutes)
  │   ├─ Find reservations in that window
  │   └─ For each reservation:
  │       ├─ Check if reminder already sent
  │       ├─ Send reminder email
  │       └─ Mark reminder as sent
  └─ Log results (processed, sent, errors)
```

### 2. Time Window Calculation

If `ReminderHoursBefore = 24`:
- Current time: 2:00 PM
- Target time: 2:00 PM tomorrow (24 hours from now)
- Window: 1:30 PM - 2:30 PM tomorrow
- Sends reminders to reservations in this 1-hour window

### 3. Email Sending Conditions

Reminder emails are sent only if:
- ✅ Business has reservations enabled
- ✅ Business has `SendReminderEmail = true`
- ✅ Reservation is in the time window
- ✅ Reservation status is "pending" or "confirmed"
- ✅ Reminder hasn't been sent yet (`ReminderSent = false`)
- ✅ Customer email is provided

## Configuration

### Business Settings

Each business can configure:

```go
type ReservationSettings struct {
    SendReminderEmail    bool // Enable/disable reminder emails
    ReminderHoursBefore  int  // Hours before reservation to send reminder (default: 24)
    SendConfirmationEmail bool // Send confirmation when reservation created
}
```

### Scheduler Configuration

Edit `internal/jobs/reservation_scheduler.go` to change the schedule:

```go
// Current: Every hour at minute 0
"0 * * * *"

// Examples:
"*/30 * * * *"  // Every 30 minutes
"0 */2 * * *"   // Every 2 hours
"0 9-17 * * *"  // Every hour between 9 AM and 5 PM
```

## Usage Examples

### Send Confirmation Email When Creating Reservation

```go
package server

import (
    "github.com/stdevmac/payverge/backend/internal/services"
    "github.com/stdevmac/payverge/backend/internal/database"
)

func CreateReservation(c *gin.Context) {
    // ... create reservation logic ...
    
    // Send confirmation email
    reminderService := services.NewReservationReminderService()
    if err := reminderService.SendConfirmationEmail(business, reservation); err != nil {
        log.Printf("Failed to send confirmation email: %v", err)
        // Don't fail the reservation creation
    }
    
    c.JSON(http.StatusCreated, reservation)
}
```

### Run One Reminder Pass Manually

The scheduler has no "run now" hook. To process reminders once (for example
in a test), call the service directly:

```go
reminders := services.NewReservationReminderService()
if err := reminders.ProcessUpcomingReservations(); err != nil {
    log.Printf("reminder pass failed: %v", err)
}
```

## Monitoring & Logging

### Log Output

The service logs detailed information:

```
Starting reservation scheduler...
Reservation scheduler started successfully
Running scheduled reservation reminder check...
Starting reservation reminder processing...
Reservation reminder processing complete. Processed: 15, Sent: 12, Errors: 0
```

### Error Handling

Errors are logged but don't stop the service:
- Failed to get businesses → Logged, next run continues
- Failed to send email → Logged, other emails still sent
- Failed to mark as sent → Logged, won't resend (time window passed)

## Testing

### 1. Test Reminder Service Directly

```go
func TestReservationReminders(t *testing.T) {
    service := services.NewReservationReminderService()
    err := service.ProcessUpcomingReservations()
    if err != nil {
        t.Errorf("Failed to process reminders: %v", err)
    }
}
```

### 2. Test with Mock Data

Create a test reservation 24 hours in the future:

```go
reservation := &database.TableReservation{
    BusinessID:      1,
    TableID:         1,
    CustomerName:    "Test Customer",
    CustomerEmail:   "test@example.com",
    PartySize:       4,
    ReservationTime: time.Now().Add(24 * time.Hour),
    Status:          "confirmed",
    ReminderSent:    false,
}
database.CreateReservation(reservation)

// Run the service
service := services.NewReservationReminderService()
service.ProcessUpcomingReservations()

// Check if reminder was sent
updated, _ := database.GetReservationByID(reservation.ID)
if !updated.ReminderSent {
    t.Error("Reminder should have been sent")
}
```

### 3. Test Scheduler

```go
func TestScheduler(t *testing.T) {
    scheduler := jobs.NewReservationScheduler()
    
    // Start scheduler
    err := scheduler.Start()
    if err != nil {
        t.Fatalf("Failed to start scheduler: %v", err)
    }
    
    // Stop scheduler
    scheduler.Stop()
}
```

## Performance Considerations

### Database Queries

The service uses indexed queries for performance:

```sql
-- Index on business_id, reservation_time, reminder_sent, status
CREATE INDEX idx_reservations_reminder 
ON table_reservations(business_id, reservation_time, reminder_sent, status);
```

### Batch Processing

- Processes all businesses in one run
- Uses efficient WHERE clauses to filter reservations
- Only loads reservations that need reminders

### Memory Usage

- Loads businesses in batches
- Processes one business at a time
- Releases memory after each business

## Troubleshooting

### Reminders Not Being Sent

1. **Check scheduler is running:**
   ```bash
   # Look for log message
   grep "Reservation scheduler started" app.log
   ```

2. **Check business settings:**
   ```sql
   SELECT * FROM reservation_settings 
   WHERE business_id = ? AND send_reminder_email = true;
   ```

3. **Check reservation data:**
   ```sql
   SELECT * FROM table_reservations 
   WHERE business_id = ? 
   AND reminder_sent = false 
   AND customer_email != '';
   ```

4. **Check time window:**
   ```go
   // Verify your reservation is in the correct time window
   reminderHours := 24
   targetTime := time.Now().Add(time.Duration(reminderHours) * time.Hour)
   log.Printf("Looking for reservations around: %v", targetTime)
   ```

### Duplicate Reminders

- Check `reminder_sent` field is being updated
- Verify database transaction commits
- Check for race conditions if running multiple instances

### Email Delivery Issues

1. **Check transactional email configuration:**
   - Verify the Resend API key and approved sender declarations are set
   - Check the local template files are present
   - Review authenticated provider events and the suppression table

2. **Check customer email:**
   ```sql
   SELECT customer_email FROM table_reservations WHERE id = ?;
   ```

3. **Test email service directly:**
   ```go
   emailServer := emails.GetEmailServer()
   err := emailServer.SendReservationReminderEmail(...)
   ```

## Best Practices

### 1. Set Appropriate Reminder Times

```go
// Good: 24 hours (gives customers time to prepare)
ReminderHoursBefore: 24

// Also good: 2 hours (last-minute reminder)
ReminderHoursBefore: 2

// Consider: Multiple reminders (requires code modification)
```

### 2. Handle Timezone Correctly

```go
// Store reservation times in UTC
reservation.ReservationTime = time.Now().UTC()

// Format for email in business timezone
loc, _ := time.LoadLocation(business.Timezone)
localTime := reservation.ReservationTime.In(loc)
```

### 3. Monitor Email Delivery

- Keep the authenticated Resend delivery webhook and suppression store healthy
- Track email delivery rates
- Alert on high error rates

### 4. Graceful Degradation

```go
// Don't fail reservation creation if email fails
if err := sendConfirmation(); err != nil {
    log.Printf("Email failed: %v", err)
    // Continue with reservation creation
}
```

## Future Enhancements

### Multiple Reminder Times

```go
// Send reminders at 24h, 2h, and 30min before
ReminderTimes: []int{24, 2, 0.5}
```

### SMS Reminders

```go
if business.SMSEnabled && customer.Phone != "" {
    sendSMSReminder(customer.Phone, reservation)
}
```

### Cancellation Reminders

```go
// Remind about cancellation deadline
if time.Until(reservation.Time) < cancellationDeadline {
    sendCancellationDeadlineReminder()
}
```

### Analytics

```go
// Track reminder effectiveness
type ReminderMetrics struct {
    Sent      int
    Opened    int
    Clicked   int
    NoShows   int
}
```

## Support

For issues or questions:
1. Check logs for error messages
2. Verify database schema is up to date
3. Test email service independently
4. Review Resend activity, delivery alerts, and suppression events
