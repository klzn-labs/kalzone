package changelogs

import (
	"context"
	"log"
)

func (u ChangelogsUsecase) Sweep(ctx context.Context) (int, error) {
	pending, err := u.store.Pending(ctx)
	if err != nil {
		return 0, err
	}
	at, retried := now(), 0
	for _, c := range pending {
		if !c.RetryDue(at) {
			continue
		}
		if err := u.Fill(ctx, c.ReleaseID); err != nil {
			log.Printf("changelog of release %s: %v", c.ReleaseID, err)
		}
		retried++
	}
	return retried, nil
}
