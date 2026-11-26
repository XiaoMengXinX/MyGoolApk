package live

import (
	"context"

	"app/internal/backend"
)

func (b *coolapkBackend) Event(ctx context.Context, id string) (backend.EventDetail, error) {
	response, err := b.client.GetRawWithCtx(ctx, "/event/detail", map[string]interface{}{"id": id})
	if err != nil {
		return backend.EventDetail{}, err
	}
	o, err := decodeAPIObject(response.Data)
	if err != nil {
		return backend.EventDetail{}, err
	}
	d := backend.EventDetail{
		Title:             firstAPIString(o, "title"),
		CoverURL:          normalizeCoolapkImage(firstAPIString(o, "logo", "pic")),
		ActionURL:         firstAPIString(o, "action_url"),
		WinnersURL:        firstAPIString(o, "prizeUserUrl"),
		RegistrationStart: apiTime(o, "time_reg_start"),
		RegistrationEnd:   apiTime(o, "time_reg_end"),
		Stage:             apiInt(o, "stage_status"),
		Joined:            apiInt(o, "is_join_status") != 0,
	}
	_, d.Description = backend.RichTextFromAPI(firstAPIString(o, "content", "description"))
	_, d.Rules = backend.RichTextFromAPI(firstAPIString(o, "notice_rule"))
	for _, user := range apiObjects(o, "sponsorUser") {
		d.Sponsors = append(d.Sponsors, authorFromObject(user))
	}
	for _, prize := range apiObjects(o, "sponsorPrize") {
		d.Prizes = append(d.Prizes, backend.CardItem{
			Title: firstAPIString(prize, "title"),
			Pic:   normalizeCoolapkImage(firstAPIString(prize, "logo")),
		})
	}
	d.Tabs, _ = topicTabsFromObject(o)
	return d, nil
}
