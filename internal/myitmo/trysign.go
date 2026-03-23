package myitmo

import (
	"context"
	"log"
)

// TrySignLesson один полный проход: все клиенты × все building_id. Успех при первой удачной записи.
func TrySignLesson(ctx context.Context, clients []*Client, signURL string, buildingIDs []int64, lessonID int64) (ok bool, userIdx int, userName string) {
	for i, cli := range clients {
		for _, bid := range buildingIDs {
			u := SignURLWithBuilding(signURL, bid)
			body, status, err := cli.SignForLesson(ctx, u, lessonID)
			if err != nil {
				log.Printf("TrySignLesson %s lesson=%d building=%d: %v", cli.Name(), lessonID, bid, err)
				continue
			}
			if status == 200 && SignSuccess(body) {
				return true, i, cli.Name()
			}
		}
	}
	return false, -1, ""
}
