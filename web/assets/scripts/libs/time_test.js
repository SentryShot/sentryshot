import { toUTC, fromUTC, fromUTC2, MILLISECOND } from "./time.js";

describe("toAndFromUTC", () => {
	test("summer", () => {
		const run = (want, timeZone) => {
			const date = new Date("2001-01-02T00:00:00.000000Z");
			const localTime = fromUTC(date, timeZone);
			const got = `DAY:${localTime.getUTCDate()} HOUR:${localTime.getUTCHours()}`;

			expect(got).toEqual(want);

			const utc = new Date(toUTC(localTime, timeZone) / MILLISECOND);
			expect(utc.getUTCDate()).toBe(2);
			expect(utc.getUTCHours()).toBe(0);
		};

		run("DAY:2 HOUR:9", "Asia/Tokyo");
		run("DAY:2 HOUR:8", "Asia/Shanghai");
		run("DAY:1 HOUR:18", "America/Mexico_City");
		run("DAY:2 HOUR:2", "Africa/Cairo");
	});
	test("winter", () => {
		const run = (want, timeZone) => {
			const date = new Date("2001-06-02T00:00:01.000000Z");
			const localTime = fromUTC(date, timeZone);
			const got = `DAY:${localTime.getUTCDate()} HOUR:${localTime.getUTCHours()}`;

			expect(got).toEqual(want);

			const utc = new Date(toUTC(localTime, timeZone) / MILLISECOND);
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
		const got = print(localTime);
		expect(got).toBe("22:4:5.6");

		const got2 = print(new Date(toUTC(localTime, "America/New_York") / MILLISECOND));
		expect(got2).toBe("3:4:5.6");
	});
	test("fromUTCerror", () => {
		let alerted = false;
		window.alert = () => {
			alerted = true;
		};

		fromUTC(new Date(), "nil");
		expect(alerted).toBe(true);
	});
	test("toUTCerror", () => {
		let alerted = false;
		window.alert = () => {
			alerted = true;
		};

		toUTC(new Date(), "nil");
		expect(alerted).toBe(true);
	});
});

describe("fromUTC2", () => {
	test("all", () => {
		const date = new Date("2001-02-03T04:05:06.000000Z");
		const got = fromUTC2(date, "Asia/Tokyo");
		const want = {
			YY: "2001",
			MM: "02",
			DD: "03",
			hh: "13",
			mm: "05",
			ss: "06",
		};
		expect(got).toEqual(want);
	});
	test("summer", () => {
		const run = (want, timezone) => {
			const date = new Date("2001-01-02T00:00:00.000000Z");
			const localTime = fromUTC2(date, timezone);
			const got = `DAY:${localTime.DD} HOUR:${localTime.hh}`;

			expect(got).toEqual(want);
		};

		run("DAY:02 HOUR:09", "Asia/Tokyo");
		run("DAY:02 HOUR:08", "Asia/Shanghai");
		run("DAY:01 HOUR:18", "America/Mexico_City");
		run("DAY:02 HOUR:02", "Africa/Cairo");
	});
	test("winter", () => {
		const run = (want, timezone) => {
			const date = new Date("2001-06-02T00:00:01.000000Z");
			const localTime = fromUTC2(date, timezone);
			const got = `DAY:${localTime.DD} HOUR:${localTime.hh}`;

			expect(got).toEqual(want);
		};
		run("DAY:02 HOUR:09", "Asia/Tokyo");
		run("DAY:02 HOUR:08", "Asia/Shanghai");
		run("DAY:01 HOUR:19", "America/Mexico_City");
		run("DAY:02 HOUR:03", "Africa/Cairo");
	});
});
