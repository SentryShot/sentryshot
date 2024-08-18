import { newTime, fromUTC, fromUTC2 } from "./time.js";

describe("Time", () => {
	test("format", () => {
		const time = newTime(Date.parse("2000-01-02T03:04:05.006Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-01-02 08:49:05.006");
	});
	test("set noop", () => {
		const time = newTime(Date.parse("2000-01-02T03:04:05.006Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-01-02 08:49:05.006");
		time.setMilliseconds(6);
		time.setSeconds(5);
		time.setMinutes(49);
		time.setHours(8);
		time.setDate(2);
		time.setMonth(0);
		time.setYear(2000);
		expect(time.format()).toBe("2000-01-02 08:49:05.006");
	});
	test("setMilliseconds", () => {
		const time = newTime(Date.parse("2000-01-02T03:04:05.006Z"), "Asia/Katmandu");
		time.setMilliseconds(123);
		expect(time.format()).toBe("2000-01-02 08:49:05.123");
	});
	test("setSeconds", () => {
		const time = newTime(Date.parse("2000-01-02T03:04:05.006Z"), "Asia/Katmandu");
		time.setSeconds(12);
		expect(time.format()).toBe("2000-01-02 08:49:12.006");
	});
	test("setMinutes", () => {
		const time = newTime(Date.parse("2000-01-01T19:04:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-01-02 00:49:00.000");

		time.setMinutes(0);
		expect(time.format()).toBe("2000-01-02 00:00:00.000");

		time.setMinutes(30);
		expect(time.format()).toBe("2000-01-02 00:30:00.000");

		time.setMinutes(59);
		expect(time.format()).toBe("2000-01-02 00:59:00.000");
	});
	test("setHours", () => {
		const time = newTime(Date.parse("2000-01-02T03:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-01-02 09:00:00.000");

		time.setHours(0);
		expect(time.format()).toBe("2000-01-02 00:00:00.000");

		time.setHours(12);
		expect(time.format()).toBe("2000-01-02 12:00:00.000");

		time.setHours(23);
		expect(time.format()).toBe("2000-01-02 23:00:00.000");
	});
	test("setDate", () => {
		const time = newTime(Date.parse("2000-01-01T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-01-02 00:00:00.000");

		time.setDate(1);
		expect(time.format()).toBe("2000-01-01 00:00:00.000");

		time.setDate(15);
		expect(time.format()).toBe("2000-01-15 00:00:00.000");

		time.setDate(31);
		expect(time.format()).toBe("2000-01-31 00:00:00.000");
	});
	test("setMonth", () => {
		const time = newTime(Date.parse("2000-01-01T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-01-02 00:00:00.000");

		time.setMonth(0);
		expect(time.format()).toBe("2000-01-02 00:00:00.000");

		time.setMonth(1);
		expect(time.format()).toBe("2000-02-02 00:00:00.000");

		time.setMonth(2);
		expect(time.format()).toBe("2000-03-02 00:00:00.000");

		time.setMonth(3);
		expect(time.format()).toBe("2000-04-02 00:00:00.000");

		time.setMonth(4);
		expect(time.format()).toBe("2000-05-02 00:00:00.000");

		time.setMonth(5);
		expect(time.format()).toBe("2000-06-02 00:00:00.000");

		time.setMonth(11);
		expect(time.format()).toBe("2000-12-02 00:00:00.000");

		time.setMonth(5);
		expect(time.format()).toBe("2000-06-02 00:00:00.000");

		time.setMonth(4);
		expect(time.format()).toBe("2000-05-02 00:00:00.000");

		time.setMonth(0);
		expect(time.format()).toBe("2000-01-02 00:00:00.000");
	});
	test("setMonth max day", () => {
		const time = newTime(Date.parse("2001-01-30T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2001-01-31 00:00:00.000");

		time.setMonth(1);
		expect(time.format()).toBe("2001-02-28 00:00:00.000");
	});
	test("setYear", () => {
		const time = newTime(Date.parse("2000-01-01T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-01-02 00:00:00.000");

		time.setYear(2005);
		expect(time.format()).toBe("2005-01-02 00:00:00.000");

		time.setYear(1995);
		expect(time.format()).toBe("1995-01-02 00:00:00.000");

		time.setYear(3000);
		expect(time.format()).toBe("3000-01-02 00:00:00.000");
	});
	test("setYear leap day", () => {
		const time = newTime(Date.parse("2000-02-28T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-02-29 00:00:00.000");

		time.setYear(2001);
		expect(time.format()).toBe("2001-02-28 00:00:00.000");
	});
	test("setYear leap day2", () => {
		const time = newTime(Date.parse("2000-02-28T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-02-29 00:00:00.000");

		time.setYear(2004);
		expect(time.format()).toBe("2004-02-29 00:00:00.000");
	});
	test("nextMonth", () => {
		const time = newTime(Date.parse("2000-10-01T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-10-02 00:00:00.000");

		time.nextMonth();
		expect(time.format()).toBe("2000-11-02 00:00:00.000");

		time.nextMonth();
		expect(time.format()).toBe("2000-12-02 00:00:00.000");

		time.nextMonth();
		expect(time.format()).toBe("2001-01-02 00:00:00.000");

		time.nextMonth();
		expect(time.format()).toBe("2001-02-02 00:00:00.000");
	});
	test("nextMonth max day", () => {
		const time = newTime(Date.parse("2001-01-30T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2001-01-31 00:00:00.000");

		time.nextMonth();
		expect(time.format()).toBe("2001-02-28 00:00:00.000");
	});
	test("prevMonth", () => {
		const time = newTime(Date.parse("2000-03-01T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-03-02 00:00:00.000");

		time.prevMonth();
		expect(time.format()).toBe("2000-02-02 00:00:00.000");

		time.prevMonth();
		expect(time.format()).toBe("2000-01-02 00:00:00.000");

		time.prevMonth();
		expect(time.format()).toBe("1999-12-02 00:00:00.000");

		time.prevMonth();
		expect(time.format()).toBe("1999-11-02 00:00:00.000");
	});
	test("prevMonth max day", () => {
		const time = newTime(Date.parse("2001-03-30T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2001-03-31 00:00:00.000");

		time.prevMonth();
		expect(time.format()).toBe("2001-02-28 00:00:00.000");
	});
	test("nextYear", () => {
		const time = newTime(Date.parse("2000-01-01T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-01-02 00:00:00.000");

		time.nextYear();
		expect(time.format()).toBe("2001-01-02 00:00:00.000");

		time.nextYear();
		expect(time.format()).toBe("2002-01-02 00:00:00.000");

		time.nextYear();
		expect(time.format()).toBe("2003-01-02 00:00:00.000");

		time.nextYear();
		expect(time.format()).toBe("2004-01-02 00:00:00.000");
	});
	test("nextYear leap day", () => {
		const time = newTime(Date.parse("2000-02-28T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-02-29 00:00:00.000");

		time.nextYear();
		expect(time.format()).toBe("2001-02-28 00:00:00.000");
	});
	test("prevYear", () => {
		const time = newTime(Date.parse("2000-01-01T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-01-02 00:00:00.000");

		time.prevYear();
		expect(time.format()).toBe("1999-01-02 00:00:00.000");

		time.prevYear();
		expect(time.format()).toBe("1998-01-02 00:00:00.000");

		time.prevYear();
		expect(time.format()).toBe("1997-01-02 00:00:00.000");

		time.prevYear();
		expect(time.format()).toBe("1996-01-02 00:00:00.000");
	});
	test("prevYear leap day", () => {
		const time = newTime(Date.parse("2000-02-28T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-02-29 00:00:00.000");

		time.prevYear();
		expect(time.format()).toBe("1999-02-28 00:00:00.000");
	});
	test("getDay", () => {
		const time = newTime(Date.parse("2000-01-01T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-01-02 00:00:00.000");
		expect(time.getDay()).toBe(0);

		time.setDate(3);
		expect(time.getDay()).toBe(1);

		time.setDate(4);
		expect(time.getDay()).toBe(2);

		time.setDate(5);
		expect(time.getDay()).toBe(3);

		time.setDate(6);
		expect(time.getDay()).toBe(4);

		time.setDate(7);
		expect(time.getDay()).toBe(5);

		time.setDate(8);
		expect(time.getDay()).toBe(6);
	});
	test("getFirstDayInMonth", () => {
		const time = newTime(Date.parse("2000-01-01T18:15:00.000Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-01-02 00:00:00.000");
		expect(time.firstDayInMonth()).toBe(6);

		time.setDate(1);
		expect(time.format()).toBe("2000-01-01 00:00:00.000");
		expect(time.getDay()).toBe(6);
	});
	test("getAll", () => {
		const time = newTime(Date.parse("2000-01-01T21:19:05.006Z"), "Asia/Katmandu");
		expect(time.format()).toBe("2000-01-02 03:04:05.006");

		expect(time.getDay()).toBe(0);
		expect(time.getMilliseconds()).toBe(6);
		expect(time.getSeconds()).toBe(5);
		expect(time.getMinutes()).toBe(4);
		expect(time.getHours()).toBe(3);
		expect(time.getDate()).toBe(2);
		expect(time.getMonth()).toBe(0);
		expect(time.getFullYear()).toBe(2000);
	});
});

describe("fromUTC", () => {
	test("summer", () => {
		const run = (expected, timeZone) => {
			const date = new Date("2001-01-02T00:00:00.000000Z");
			const localTime = fromUTC(date, timeZone);
			const actual = `DAY:${localTime.getUTCDate()} HOUR:${localTime.getUTCHours()}`;

			expect(actual).toEqual(expected);
		};

		run("DAY:2 HOUR:9", "Asia/Tokyo");
		run("DAY:2 HOUR:8", "Asia/Shanghai");
		run("DAY:1 HOUR:18", "America/Mexico_City");
		run("DAY:2 HOUR:2", "Africa/Cairo");
	});
	test("winter", () => {
		const run = (expected, timeZone) => {
			const date = new Date("2001-06-02T00:00:01.000000Z");
			const localTime = fromUTC(date, timeZone);
			const actual = `DAY:${localTime.getUTCDate()} HOUR:${localTime.getUTCHours()}`;

			expect(actual).toEqual(expected);
		};
		run("DAY:2 HOUR:9", "Asia/Tokyo");
		run("DAY:2 HOUR:8", "Asia/Shanghai");
		run("DAY:1 HOUR:19", "America/Mexico_City");
		run("DAY:2 HOUR:3", "Africa/Cairo");
	});
	test("milliseconds", () => {
		const date = new Date("2001-01-02T03:04:05.006000Z");
		const localTime = fromUTC(date, "America/New_York");
		const print = (d) => {
			return (
				d.getUTCHours() +
				":" +
				d.getUTCMinutes() +
				":" +
				d.getUTCSeconds() +
				"." +
				d.getUTCMilliseconds()
			);
		};
		const actual = print(localTime);
		const expected = "22:4:5.6";
		expect(actual).toEqual(expected);
	});
	test("fromUTCerror", () => {
		let alerted = false;
		window.alert = () => {
			alerted = true;
		};

		const date = new Date("2001-01-02T03:04:05.006+00:00");
		fromUTC(date, "nil");
		expect(alerted).toBe(true);
	});
});

describe("fromUTC2", () => {
	test("all", () => {
		const date = new Date("2001-02-03T04:05:06.000000Z");
		const actual = fromUTC2(date, "Asia/Tokyo");
		const expected = {
			YY: "2001",
			MM: "02",
			DD: "03",
			hh: "13",
			mm: "05",
			ss: "06",
		};
		expect(actual).toEqual(expected);
	});
	test("summer", () => {
		const run = (expected, timezone) => {
			const date = new Date("2001-01-02T00:00:00.000000Z");
			const localTime = fromUTC2(date, timezone);
			const actual = `DAY:${localTime.DD} HOUR:${localTime.hh}`;

			expect(actual).toEqual(expected);
		};

		run("DAY:02 HOUR:09", "Asia/Tokyo");
		run("DAY:02 HOUR:08", "Asia/Shanghai");
		run("DAY:01 HOUR:18", "America/Mexico_City");
		run("DAY:02 HOUR:02", "Africa/Cairo");
	});
	test("winter", () => {
		const run = (expected, timezone) => {
			const date = new Date("2001-06-02T00:00:01.000000Z");
			const localTime = fromUTC2(date, timezone);
			const actual = `DAY:${localTime.DD} HOUR:${localTime.hh}`;

			expect(actual).toEqual(expected);
		};
		run("DAY:02 HOUR:09", "Asia/Tokyo");
		run("DAY:02 HOUR:08", "Asia/Shanghai");
		run("DAY:01 HOUR:19", "America/Mexico_City");
		run("DAY:02 HOUR:03", "Africa/Cairo");
	});
});
