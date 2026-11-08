package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-jobs/internal/store"
)

// ---- offers ----
//
// One offer per job, to weigh side by side: the pay for a year (a monthly
// or hourly one counted up), the bonus, the time off and what else comes
// with it, and the day to decide by. Adding one moves the job to Offer;
// accepting one moves it to Hired.

func (s *Server) handleListOffers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListOffers(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "offers")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type offerIn struct {
	Base      *float64 `json:"base_amount"`
	Currency  *string  `json:"currency"`
	Period    *string  `json:"period"`
	Bonus     *float64 `json:"bonus_amount"`
	Equity    *string  `json:"equity"`
	PTODays   *int     `json:"pto_days"`
	Benefits  *string  `json:"benefits"`
	StartDate *string  `json:"start_date"`
	DecideBy  *string  `json:"decide_by"`
	Status    *string  `json:"status"`
	Notes     *string  `json:"notes"`
}

func (in offerIn) apply(o *store.Offer) error {
	var err error
	if in.Base != nil {
		if o.Base, err = cents(*in.Base); err != nil {
			return err
		}
	}
	if in.Bonus != nil {
		if o.Bonus, err = cents(*in.Bonus); err != nil {
			return err
		}
	}
	if in.Currency != nil {
		if o.Currency, err = normCurrency(*in.Currency); err != nil {
			return err
		}
	}
	if in.Period != nil {
		p := strings.ToLower(strings.TrimSpace(*in.Period))
		if !oneOf(p, store.Periods) {
			return &userErr{"the pay is per year, month or hour"}
		}
		o.Period = p
	}
	if in.PTODays != nil {
		if *in.PTODays < 0 || *in.PTODays > 365 {
			return &userErr{"days off are 0 to 365"}
		}
		o.PTODays = *in.PTODays
	}
	txt := func(p *string, dst *string, n int) {
		if p != nil {
			*dst = clip(strings.TrimSpace(*p), n)
		}
	}
	txt(in.Equity, &o.Equity, 300)
	txt(in.Benefits, &o.Benefits, 5000)
	txt(in.StartDate, &o.StartDate, 10)
	txt(in.DecideBy, &o.DecideBy, 10)
	txt(in.Notes, &o.Notes, 10000)
	if !validDate(o.StartDate) || !validDate(o.DecideBy) {
		return &userErr{"days are YYYY-MM-DD"}
	}
	if in.Status != nil {
		v := strings.ToLower(strings.TrimSpace(*in.Status))
		if !oneOf(v, store.OfferStatuses) {
			return &userErr{"an offer is open, negotiating, accepted or declined"}
		}
		o.Status = v
	}
	return nil
}

// handleSaveOffer adds or changes the offer for a job.
func (s *Server) handleSaveOffer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	j, err := s.store.GetJob(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "job")
		return
	}
	var in offerIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	o, err := s.store.OfferFor(ctx, uid, j.ID)
	isNew := err != nil
	if isNew {
		o = &store.Offer{UserID: uid, JobID: j.ID, Currency: j.Currency, Period: j.SalaryPeriod, Status: "open"}
	}
	oldStatus := o.Status
	if err := in.apply(o); err != nil {
		s.userFail(w, err, "offer")
		return
	}
	o.UserID, o.JobID = uid, j.ID
	if err := s.store.SaveOffer(ctx, o); err != nil {
		s.fail(w, err, "offer")
		return
	}
	switch {
	case isNew:
		_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, JobID: j.ID, Kind: "offer", Title: "Offer: " + offerWords(o)})
		if j.Status == store.StatusOpen && store.Rank(j.StageKind) < 3 {
			if sg, err := s.store.FirstStage(ctx, uid, store.KindOffer); err == nil {
				_, _ = s.moveJob(ctx, uid, j.ID, sg.ID, nil, nil, "", "")
			}
		} else {
			payload := s.jobEvent(j)
			payload["offer"] = offerWords(o)
			s.publish(ctx, uid, "jobs.offer_received", fmt.Sprintf("job:%d", j.ID), payload)
		}
	case o.Status != oldStatus:
		_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, JobID: j.ID, Kind: "offer", Title: map[string]string{"open": "Offer open again",
			"negotiating": "Negotiating the offer", "accepted": "Accepted the offer", "declined": "Declined the offer"}[o.Status]})
	}
	if o.Status != oldStatus {
		switch o.Status {
		case "accepted":
			if sg, err := s.store.FirstStage(ctx, uid, store.KindHired); err == nil && j.Status != store.StatusHired {
				_, _ = s.moveJob(ctx, uid, j.ID, sg.ID, nil, nil, "", "")
			}
		case "declined":
			if sg, err := s.store.FirstStage(ctx, uid, store.KindClosed); err == nil && j.Status == store.StatusOpen {
				_, _ = s.moveJob(ctx, uid, j.ID, sg.ID, nil, nil, "withdrew", "Declined the offer")
			}
		}
	}
	o, _ = s.store.OfferFor(ctx, uid, j.ID)
	writeJSON(w, http.StatusOK, o)
}

// offerWords is an offer in a few words: "62,000 EUR a year + 5,000 bonus".
func offerWords(o *store.Offer) string {
	per := map[string]string{"year": "a year", "month": "a month", "hour": "an hour"}[o.Period]
	out := strings.TrimSpace(fmtMoney(o.Base, o.Currency) + " " + per)
	if o.Base == 0 {
		out = "no pay written yet"
	}
	if o.Bonus > 0 {
		out += " + " + fmtMoney(o.Bonus, o.Currency) + " bonus"
	}
	return out
}

func (s *Server) handleDeleteOffer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteOffer(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "offer")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
