// SheetsSync Lambda: triggered by EventBridge every 5 minutes. Pulls
// character sheets from Google Sheets, compares a content hash against
// sheet_sync_state, and upserts characters/weapons only when something
// changed. Skip-path is cheap: one Sheets API call + one indexed SELECT.
//
// TODO(port): implement the actual parsing from app/Console/Commands/
// GetDataFromSheets.php and app/Services/SheetsService.php.
package main

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/lambda"
)

func handler(ctx context.Context) error {
	log.Println("sheetssync: not yet implemented")
	return nil
}

func main() {
	lambda.Start(handler)
}
