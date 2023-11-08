// AUTOGENERATED BY private/apigen
// DO NOT EDIT.

class APIError extends Error {
    constructor(
        public readonly msg: string,
        public readonly responseStatusCode?: number,
    ) {
        super(msg);
    }
}

export class ExampleHttpApiV1 {
    public readonly respStatusCode: number;

    // When respStatuscode is passed, the client throws an APIError on each method call
    // with respStatusCode as HTTP status code.
    // respStatuscode must be equal or greater than 400
    constructor(respStatusCode?: number) {
        if (typeof respStatusCode === 'undefined') {
            this.respStatusCode = 0;
            return;
        }

        if (respStatusCode < 400) {
            throw new Error('invalid response status code for API Error, it must be greater or equal than 400');
        }

        this.respStatusCode = respStatusCode;
    }

    public async getExamples(): Promise<string[]> {
        if (this.respStatusCode !== 0) {
            throw new APIError('mock error message: ' + this.respStatusCode, this.respStatusCode);
        }

        return JSON.parse('["example-1","example-2","example-3"]') as string[];
    }
}