import { toUTC, fromUTC, fromUTC2 } from "./time.js";

describe("toAndFromUTC", () => {
	test("summer", () => {
		const run = (expected, timeZone) => {
			const date = new Date("2001-01-02T00:00:00.000000Z");
			const localTime = fromUTC(date, timeZone);
			const actual = `DAY:${localTime.getUTCDate()} HOUR:${localTime.getUTCHours()}`;

			expect(actual).toEqual(expected);

			const utc = toUTC(localTime, timeZone);
			expect(utc.getUTCDate()).toBe(2);
			expect(utc.getUTCHours()).toBe(0);
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

			const utc = toUTC(localTime, timeZone);
			expect(utc.getUTCDate()).toBe(2);
			expect(utc.getUTCHours()).toBe(0);
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

		const utc = toUTC(localTime, "America/New_York");
		const actual2 = print(utc);
		const expected2 = "3:4:5.6";
		expect(actual2).toEqual(expected2);
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
	test("toUTCerror", () => {
		let alerted = false;
		window.alert = () => {
			alerted = true;
		};

		const date = new Date("2001-01-02T03:04:05.006+00:00");
		toUTC(date, "nil");
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
