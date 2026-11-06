package inboxrepository

import (
	"tg-bot-proxy/internal/domain/repository"
	"tg-bot-proxy/internal/generated/xo"

	sq "github.com/Masterminds/squirrel"
)

func inboxProcessorLeaseExists(lease repository.Lease) sq.SelectBuilder {
	return sq.Select("1").
		From(xo.Table_InboxProcessorLock_With_Alias).
		Where(sq.Eq{
			xo.Field_InboxProcessorLock_BotID_With_Alias:        lease.BotID,
			xo.Field_InboxProcessorLock_LeaseOwner_With_Alias:   lease.Owner,
			xo.Field_InboxProcessorLock_LeaseVersion_With_Alias: lease.Version,
		}).
		Where(sq.Expr(
			xo.Field_InboxProcessorLock_LeaseUntil_With_Alias + " > clock_timestamp()",
		))
}
