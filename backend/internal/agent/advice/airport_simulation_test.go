package advice

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"aviation-journey-agent/backend/internal/domain"
)

func departureLoad(current, total int) string {
	return fmt.Sprintf("客流（出发）：当前小时计划旅客座位数 %d 座，未来 24 小时合计约 %d 座", current, total)
}

func TestSimulateAirportTimes_PlannedDepartureLoad(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		current                 int
		baggage                 bool
		walk, checkIn, security int
	}{
		{"低客流不缩短基准", 500, false, 10, 15, 20},
		{"小时均值", 1000, false, 10, 15, 20},
		{"较高客流向上取整", 1500, false, 15, 23, 30},
		{"托运增加柜台时间", 1500, true, 15, 38, 30},
		{"高峰倍率封顶", 5000, true, 20, 50, 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line := departureLoad(tc.current, 24000)
			got, ok := simulateAirportTimes([]string{
				"客流（到达）：当前小时计划旅客座位数 20000 座，未来 24 小时合计约 24000 座",
				line,
			}, tc.baggage)
			if !ok || got.walkMin != tc.walk || got.checkInMin != tc.checkIn || got.securityMin != tc.security || got.source != line {
				t.Fatalf("模拟结果 = %+v, ok=%v", got, ok)
			}
		})
	}
}

func TestSimulateAirportTimes_RejectsMissingOrInvalidLoad(t *testing.T) {
	for _, line := range []string{
		"",
		"客流（到达）：当前小时计划旅客座位数 1000 座，未来 24 小时合计约 24000 座",
		"客流（出发）：当前小时数据暂缺，未来 24 小时合计约 24000 座",
		"客流（出发）：当前小时旅客人数 1000 人，未来 24 小时合计约 24000 人",
		departureLoad(0, 24000),
		departureLoad(1000, 0),
		departureLoad(1000, 500),
		departureLoad(-1, 24000),
		"客流（出发）：当前小时计划旅客座位数 999999999999999999999999 座，未来 24 小时合计约 24000 座",
	} {
		t.Run(line, func(t *testing.T) {
			if got, ok := simulateAirportTimes([]string{line}, true); ok {
				t.Fatalf("缺失或非法客流不得产生模拟：%+v", got)
			}
		})
	}
}

func TestEvaluate_SimulationChangesActionDetailOnly(t *testing.T) {
	client := &fakeClient{content: `{
		"stage": "en_route", "risk": "yellow", "alert": "先到机场办理托运",
		"cards": [{"label": "预计到达机场", "value": "52 分钟"}],
		"actions": [
			{"id": "leave_now", "title": "前往机场", "detail": "到机场先办理托运再安检", "nav": "airport"},
			{"id": "documents", "title": "准备证件", "detail": "提前准备身份证", "nav": null}
		],
		"reasons": ["航班状态正常"]
	}`}
	state := readyState()
	state.Guide = []string{departureLoad(1500, 24000)}
	state.UpdatedAt = "2026-10-13T10:00:00+08:00"
	originalState := state
	got := NewAgent(client).Evaluate(context.Background(), Input{
		State: state, HasBaggage: true, AirportIATA: "PVG",
	})
	if got.Stage != domain.StageEnRoute || got.Risk != domain.RiskYellow || got.Alert == nil || *got.Alert != "先到机场办理托运" {
		t.Fatalf("模拟不应改变阶段、风险或提醒：%+v", got)
	}
	if !reflect.DeepEqual(got.Cards, []domain.Card{{Label: "预计到达机场", Value: "52 分钟"}}) {
		t.Fatalf("不得新增模拟卡片：%+v", got.Cards)
	}
	if len(got.Actions) != 2 || got.Actions[0].ID != "leave_now" || got.Actions[0].Title != "前往机场" || got.Actions[0].Nav == nil || got.Actions[1].Detail != "提前准备身份证" {
		t.Fatalf("应只修改相关行动说明，保留顺序和导航：%+v", got.Actions)
	}
	for _, want := range []string{"到机场先办理托运再安检", "模拟估算（非实测）", "\n值机时间（模拟）：约 38 分钟", "\n安检时间（模拟）：约 30 分钟", "\n步行时间（模拟）：约 15 分钟"} {
		if !strings.Contains(got.Actions[0].Detail, want) {
			t.Errorf("行动说明缺少 %q：%s", want, got.Actions[0].Detail)
		}
	}
	for _, want := range []string{departureLoad(1500, 24000), "模拟基准", "不代表真实耗时", "未用于风险或缓冲计算", state.UpdatedAt} {
		if !hasReason(got.Reasons, want) {
			t.Errorf("依据缺少 %q：%v", want, got.Reasons)
		}
	}
	if !reflect.DeepEqual(state, originalState) {
		t.Fatal("模拟不能改写原始状态")
	}
}

func TestWithAirportTimeSimulation_RespectsStageAndBaggage(t *testing.T) {
	state := readyState()
	state.Guide = []string{departureLoad(1000, 24000)}
	for _, tc := range []struct {
		stage                   string
		baggage                 bool
		checkIn, security, walk bool
	}{
		{domain.StageUnknown, false, true, true, true},
		{domain.StageEnRoute, true, true, true, true},
		{domain.StageAtAirport, false, true, true, true},
		{domain.StageCheckIn, true, true, true, true},
		{domain.StageSecurity, true, false, true, true},
		{domain.StageWaiting, true, false, false, true},
		{domain.StageBoarding, true, false, false, false},
		{domain.StageDeparted, true, false, false, false},
		{domain.StageDisrupted, true, false, false, false},
	} {
		t.Run(tc.stage, func(t *testing.T) {
			base := domain.NewAdvice()
			base.Stage = tc.stage
			base.Actions = []domain.Action{{ID: "airport", Title: "前往登机口", Detail: "按现场指引行走"}}
			got := WithAirportTimeSimulation(base, Input{State: state, HasBaggage: tc.baggage})
			detail := got.Actions[0].Detail
			if strings.Contains(detail, "值机时间（模拟）") != tc.checkIn || strings.Contains(detail, "安检时间（模拟）") != tc.security || strings.Contains(detail, "步行时间（模拟）") != tc.walk {
				t.Fatalf("阶段应只提示未完成流程：%s", detail)
			}
			if tc.checkIn && !tc.baggage && (!strings.Contains(detail, "仅需柜台值机时预留") || strings.Contains(detail, "托运")) {
				t.Fatalf("无托运时值机须为条件提示，不得增加托运步骤：%s", detail)
			}
		})
	}
}

func TestWithAirportTimeSimulation_RefreshDoesNotMutateCachedAdvice(t *testing.T) {
	state := readyState()
	state.Guide = []string{departureLoad(1000, 24000)}
	base := domain.NewAdvice()
	base.Stage = domain.StageAtAirport
	base.Actions = []domain.Action{{ID: "security", Title: "前往安检", Detail: "按指引前往安检后去登机口"}}
	in := Input{State: state, HasBaggage: true}
	previous := WithAirportTimeSimulation(base, in)
	if got := WithAirportTimeSimulation(previous, in); !reflect.DeepEqual(got, previous) {
		t.Fatalf("重复刷新不应累积模拟文案：%+v", got)
	}

	in.State.Guide = []string{departureLoad(2000, 24000)}
	next := WithAirportTimeSimulation(previous, in)
	if !strings.Contains(next.Actions[0].Detail, "安检时间（模拟）：约 40 分钟") || strings.Contains(next.Actions[0].Detail, "安检时间（模拟）：约 20 分钟") {
		t.Fatalf("新客流应替换旧模拟：%s", next.Actions[0].Detail)
	}
	if !strings.Contains(previous.Actions[0].Detail, "安检时间（模拟）：约 20 分钟") || len(previous.Reasons) != 1 || !hasReason(previous.Reasons, departureLoad(1000, 24000)) {
		t.Fatalf("旧快照被意外修改：%+v", previous)
	}

	in.State.Guide = []string{"客流（出发）：当前小时数据暂缺，未来 24 小时合计约 24000 座"}
	missing := WithAirportTimeSimulation(previous, in)
	if missing.Actions[0].Detail != base.Actions[0].Detail || !hasReason(missing.Reasons, "暂不提供") || hasReason(missing.Reasons, "模拟基准") {
		t.Fatalf("客流缺失应去除旧模拟并说明原因：%+v", missing)
	}
}

func TestWithAirportTimeSimulation_PrefersProcedureOverLocationAction(t *testing.T) {
	state := readyState()
	state.Guide = []string{departureLoad(1000, 24000)}
	base := domain.NewAdvice()
	base.Actions = []domain.Action{
		{ID: "location", Title: "请开启定位或手动输入当前位置", Detail: "缺少定位，无法估算到机场耗时和剩余缓冲时间"},
		{ID: "check_in", Title: "提前办理值机并托运行李", Detail: "先去值机柜台办理手续"},
	}
	got := WithAirportTimeSimulation(base, Input{State: state, HasBaggage: true})
	if got.Actions[0].Detail != base.Actions[0].Detail || !strings.Contains(got.Actions[1].Detail, "\n值机时间（模拟）：约 25 分钟") {
		t.Fatalf("流程模拟应归属具体机场行动，不应挂在定位提示下：%+v", got.Actions)
	}
}

func TestWithAirportTimeSimulation_SkipsDegradedAndFinishedAdvice(t *testing.T) {
	state := readyState()
	state.Guide = []string{departureLoad(1000, 24000)}
	if got := NewAgent(nil).Evaluate(context.Background(), Input{State: state}); len(got.Actions) != 0 || len(got.Cards) != 0 {
		t.Fatalf("模型降级不应新增内容：%+v", got)
	}
	base := domain.NewAdvice()
	base.Stage = domain.StageEnRoute
	base.Actions = []domain.Action{{ID: "airport", Title: "前往机场", Detail: "先去机场"}}
	for _, status := range []string{domain.FlightStatusDeparted, domain.FlightStatusCancelled} {
		state.FlightStatus = status
		got := WithAirportTimeSimulation(base, Input{State: state})
		if !reflect.DeepEqual(got, base) {
			t.Fatalf("航班 %s 时不应提示机场流程：%+v", status, got)
		}
	}
	if got := WithAirportTimeSimulation(base, Input{State: domain.NewState()}); !reflect.DeepEqual(got, base) {
		t.Fatalf("无航班数据时不得补充模拟：%+v", got)
	}
}
