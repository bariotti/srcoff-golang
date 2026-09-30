package service

import "time"

// DescricaoDiasNaoUteis descreve, para mensagens de erro, o que é considerado dia não útil.
const DescricaoDiasNaoUteis = "sábado, domingo, Natal (25/12) ou Ano Novo (01/01)"

// EhDiaUtil informa se a data é um dia útil. São considerados NÃO úteis:
// sábado, domingo, Natal (25/12) e Ano Novo (01/01) — independentemente do ano.
func EhDiaUtil(t time.Time) bool {
	switch t.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	}
	if t.Month() == time.December && t.Day() == 25 {
		return false // Natal
	}
	if t.Month() == time.January && t.Day() == 1 {
		return false // Ano Novo
	}
	return true
}

// DiaUtilAnterior retorna o dia útil imediatamente anterior a t (pulando fins de
// semana e feriados). Ex.: segunda-feira → sexta-feira anterior.
func DiaUtilAnterior(t time.Time) time.Time {
	d := t.AddDate(0, 0, -1)
	for !EhDiaUtil(d) {
		d = d.AddDate(0, 0, -1)
	}
	return d
}
