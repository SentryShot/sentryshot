function toUTC(date, timeZone) {
	const tmp1 = new Date(date.toLocaleString("en-US", { timeZone: "utc" }));
	const unix1 = tmp1.getTime();

	const tmp2 = new Date(date.toLocaleString("en-US", { timeZone: timeZone }));
	const unix2 = tmp2.getTime();

	let utc;
	if (unix1 > unix2) {
		const offset = unix1 - unix2;
		utc = new Date(date.getTime() + offset);
	} else {
		const offset = unix2 - unix1;
		utc = new Date(date.getTime() - offset);
	}
	utc.setMilliseconds(date.getMilliseconds());
	return utc;
}

function fromUTC(date, timeZone) {
	try {
		const localTime = new Date(date.toLocaleString("en-US", { timeZone: timeZone }));
		localTime.setMilliseconds(date.getMilliseconds());
		return localTime;
	} catch (error) {
		alert(error);
	}
}

export { toUTC, fromUTC };
