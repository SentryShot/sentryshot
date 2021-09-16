import { toUTC, fromUTC } from "./time.mjs";

describe("toAndFromUTC", () => {
	test("summer", () => {
		const run = (expected, timeZone) => {
			const date = new Date("2001-01-02T00:00:00+00:00");
			const localTime = fromUTC(date, timeZone);
			const actual = `DAY:${localTime.getDate()} HOUR:${localTime.getHours()}`;

			expect(actual).toEqual(expected);

			const utc = toUTC(localTime, timeZone);
			expect(utc.getDate()).toEqual(2);
			expect(utc.getHours()).toEqual(0);
		};

		run("DAY:2 HOUR:9", "Asia/Tokyo");
		run("DAY:2 HOUR:8", "Asia/Shanghai");
		run("DAY:1 HOUR:18", "America/Mexico_City");
		run("DAY:2 HOUR:2", "Africa/Cairo");
	});
	test("winter", () => {
		const run = (expected, timeZone) => {
			const date = new Date("2001-06-02T00:00:01+00:00");
			const localTime = fromUTC(date, timeZone);
			const actual = `DAY:${localTime.getDate()} HOUR:${localTime.getHours()}`;

			expect(actual).toEqual(expected);

			const utc = toUTC(localTime, timeZone);
			expect(utc.getDate()).toEqual(2);
			expect(utc.getHours()).toEqual(0);
		};
		run("DAY:2 HOUR:9", "Asia/Tokyo");
		run("DAY:2 HOUR:8", "Asia/Shanghai");
		run("DAY:1 HOUR:19", "America/Mexico_City");
		run("DAY:2 HOUR:3", "Africa/Cairo");
	});
	test("milliseconds", () => {
		const date = new Date("2001-01-02T03:04:05.006+00:00");
		const localTime = fromUTC(date, "America/New_York");
		const print = (d) => {
			return (
				d.getHours() +
				":" +
				d.getMinutes() +
				":" +
				d.getSeconds() +
				"." +
				d.getMilliseconds()
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
	test("error", () => {
		let alerted = false;
		window.alert = () => {
			alerted = true;
		};

		window.fetch = {
			status: 400,
			text() {
				return "";
			},
		};
		const date = new Date("2001-01-02T03:04:05.006+00:00");
		fromUTC(date, "nil");
		expect(alerted).toEqual(true);
	});
});
