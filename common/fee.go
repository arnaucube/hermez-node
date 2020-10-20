package common

import (
	"math"
	"math/big"
)

// RecommendedFee is the recommended fee to pay in USD per transaction set by
// the coordinator according to the tx type (if the tx requires to create an
// account and register, only register or he account already esists)
type RecommendedFee struct {
	ExistingAccount           float64
	CreatesAccount            float64
	CreatesAccountAndRegister float64
}

// FeeSelector is used to select a percentage from the FeePlan.
type FeeSelector uint8

// Percentage returns the associated percentage of the FeeSelector
func (f FeeSelector) Percentage() float64 {
	if f == 0 {
		return 0
		//nolint:gomnd
	} else if f <= 32 { //nolint:gomnd
		return math.Pow(10, -24+(float64(f)/2)) //nolint:gomnd
	} else if f <= 223 { //nolint:gomnd
		return math.Pow(10, -8+(0.041666666666667*(float64(f)-32))) //nolint:gomnd
	} else {
		return math.Pow(10, float64(f)-224) //nolint:gomnd
	}
}

// MaxFeePlan is the maximum value of the FeePlan
const MaxFeePlan = 256

// FeePlan represents the fee model, a position in the array indicates the
// percentage of tokens paid in concept of fee for a transaction
var FeePlan = [MaxFeePlan]float64{}

// CalcFeeAmount calculates the fee amount in tokens from an amount and
// feeSelector (fee index).
func CalcFeeAmount(amount *big.Int, feeSel FeeSelector) *big.Int {
	feeAmount := new(big.Int).Mul(amount, FeeFactorLsh79[int(feeSel)])
	return feeAmount.Rsh(feeAmount, 79)
}

func init() {
	setFeeFactorLsh79(&FeeFactorLsh79)
}

// FeeFactorLsh79 is the feeFactor << 79
var FeeFactorLsh79 [256]*big.Int

func setFeeFactorLsh79(feeFactorLsh79 *[256]*big.Int) {
	feeFactorLsh79[0], _ = new(big.Int).SetString("0", 10)
	feeFactorLsh79[1], _ = new(big.Int).SetString("1", 10)
	feeFactorLsh79[2], _ = new(big.Int).SetString("6", 10)
	feeFactorLsh79[3], _ = new(big.Int).SetString("19", 10)
	feeFactorLsh79[4], _ = new(big.Int).SetString("60", 10)
	feeFactorLsh79[5], _ = new(big.Int).SetString("191", 10)
	feeFactorLsh79[6], _ = new(big.Int).SetString("604", 10)
	feeFactorLsh79[7], _ = new(big.Int).SetString("1911", 10)
	feeFactorLsh79[8], _ = new(big.Int).SetString("6044", 10)
	feeFactorLsh79[9], _ = new(big.Int).SetString("19114", 10)
	feeFactorLsh79[10], _ = new(big.Int).SetString("60446", 10)
	feeFactorLsh79[11], _ = new(big.Int).SetString("191147", 10)
	feeFactorLsh79[12], _ = new(big.Int).SetString("604462", 10)
	feeFactorLsh79[13], _ = new(big.Int).SetString("1911479", 10)
	feeFactorLsh79[14], _ = new(big.Int).SetString("6044629", 10)
	feeFactorLsh79[15], _ = new(big.Int).SetString("19114795", 10)
	feeFactorLsh79[16], _ = new(big.Int).SetString("60446290", 10)
	feeFactorLsh79[17], _ = new(big.Int).SetString("191147955", 10)
	feeFactorLsh79[18], _ = new(big.Int).SetString("604462909", 10)
	feeFactorLsh79[19], _ = new(big.Int).SetString("1911479556", 10)
	feeFactorLsh79[20], _ = new(big.Int).SetString("6044629098", 10)
	feeFactorLsh79[21], _ = new(big.Int).SetString("19114795560", 10)
	feeFactorLsh79[22], _ = new(big.Int).SetString("60446290980", 10)
	feeFactorLsh79[23], _ = new(big.Int).SetString("191147955608", 10)
	feeFactorLsh79[24], _ = new(big.Int).SetString("604462909807", 10)
	feeFactorLsh79[25], _ = new(big.Int).SetString("1911479556084", 10)
	feeFactorLsh79[26], _ = new(big.Int).SetString("6044629098073", 10)
	feeFactorLsh79[27], _ = new(big.Int).SetString("19114795560840", 10)
	feeFactorLsh79[28], _ = new(big.Int).SetString("60446290980731", 10)
	feeFactorLsh79[29], _ = new(big.Int).SetString("191147955608404", 10)
	feeFactorLsh79[30], _ = new(big.Int).SetString("604462909807314", 10)
	feeFactorLsh79[31], _ = new(big.Int).SetString("1911479556084045", 10)
	feeFactorLsh79[32], _ = new(big.Int).SetString("6044629098073146", 10)
	feeFactorLsh79[33], _ = new(big.Int).SetString("6653288015630616", 10)
	feeFactorLsh79[34], _ = new(big.Int).SetString("7323235338466790", 10)
	feeFactorLsh79[35], _ = new(big.Int).SetString("8060642451758603", 10)
	feeFactorLsh79[36], _ = new(big.Int).SetString("8872302163198820", 10)
	feeFactorLsh79[37], _ = new(big.Int).SetString("9765691276621298", 10)
	feeFactorLsh79[38], _ = new(big.Int).SetString("10749039466425620", 10)
	feeFactorLsh79[39], _ = new(big.Int).SetString("11831405087254648", 10)
	feeFactorLsh79[40], _ = new(big.Int).SetString("13022758617264914", 10)
	feeFactorLsh79[41], _ = new(big.Int).SetString("14334074503647984", 10)
	feeFactorLsh79[42], _ = new(big.Int).SetString("15777432256460256", 10)
	feeFactorLsh79[43], _ = new(big.Int).SetString("17366127722012378", 10)
	feeFactorLsh79[44], _ = new(big.Int).SetString("19114795560840448", 10)
	feeFactorLsh79[45], _ = new(big.Int).SetString("21039544058494708", 10)
	feeFactorLsh79[46], _ = new(big.Int).SetString("23158103510989148", 10)
	feeFactorLsh79[47], _ = new(big.Int).SetString("25489989551801104", 10)
	feeFactorLsh79[48], _ = new(big.Int).SetString("28056682924947216", 10)
	feeFactorLsh79[49], _ = new(big.Int).SetString("30881827360160752", 10)
	feeFactorLsh79[50], _ = new(big.Int).SetString("33991447372945972", 10)
	feeFactorLsh79[51], _ = new(big.Int).SetString("37414187995827888", 10)
	feeFactorLsh79[52], _ = new(big.Int).SetString("41181578649142096", 10)
	feeFactorLsh79[53], _ = new(big.Int).SetString("45328323582075176", 10)
	feeFactorLsh79[54], _ = new(big.Int).SetString("49892621559424256", 10)
	feeFactorLsh79[55], _ = new(big.Int).SetString("54916517738950528", 10)
	feeFactorLsh79[56], _ = new(big.Int).SetString("60446290980731456", 10)
	feeFactorLsh79[57], _ = new(big.Int).SetString("66532880156306032", 10)
	feeFactorLsh79[58], _ = new(big.Int).SetString("73232353384667904", 10)
	feeFactorLsh79[59], _ = new(big.Int).SetString("80606424517586032", 10)
	feeFactorLsh79[60], _ = new(big.Int).SetString("88723021631988016", 10)
	feeFactorLsh79[61], _ = new(big.Int).SetString("97656912766212992", 10)
	feeFactorLsh79[62], _ = new(big.Int).SetString("107490394664256192", 10)
	feeFactorLsh79[63], _ = new(big.Int).SetString("118314050872546240", 10)
	feeFactorLsh79[64], _ = new(big.Int).SetString("130227586172649136", 10)
	feeFactorLsh79[65], _ = new(big.Int).SetString("143340745036479856", 10)
	feeFactorLsh79[66], _ = new(big.Int).SetString("157774322564602240", 10)
	feeFactorLsh79[67], _ = new(big.Int).SetString("173661277220123776", 10)
	feeFactorLsh79[68], _ = new(big.Int).SetString("191147955608404480", 10)
	feeFactorLsh79[69], _ = new(big.Int).SetString("210395440584946624", 10)
	feeFactorLsh79[70], _ = new(big.Int).SetString("231581035109891488", 10)
	feeFactorLsh79[71], _ = new(big.Int).SetString("254899895518011040", 10)
	feeFactorLsh79[72], _ = new(big.Int).SetString("280566829249471584", 10)
	feeFactorLsh79[73], _ = new(big.Int).SetString("308818273601607552", 10)
	feeFactorLsh79[74], _ = new(big.Int).SetString("339914473729459712", 10)
	feeFactorLsh79[75], _ = new(big.Int).SetString("374141879958278144", 10)
	feeFactorLsh79[76], _ = new(big.Int).SetString("411815786491420928", 10)
	feeFactorLsh79[77], _ = new(big.Int).SetString("453283235820751744", 10)
	feeFactorLsh79[78], _ = new(big.Int).SetString("498926215594241536", 10)
	feeFactorLsh79[79], _ = new(big.Int).SetString("549165177389505280", 10)
	feeFactorLsh79[80], _ = new(big.Int).SetString("604462909807314560", 10)
	feeFactorLsh79[81], _ = new(big.Int).SetString("665328801563060352", 10)
	feeFactorLsh79[82], _ = new(big.Int).SetString("732323533846679040", 10)
	feeFactorLsh79[83], _ = new(big.Int).SetString("806064245175860224", 10)
	feeFactorLsh79[84], _ = new(big.Int).SetString("887230216319880192", 10)
	feeFactorLsh79[85], _ = new(big.Int).SetString("976569127662129920", 10)
	feeFactorLsh79[86], _ = new(big.Int).SetString("1074903946642562048", 10)
	feeFactorLsh79[87], _ = new(big.Int).SetString("1183140508725462528", 10)
	feeFactorLsh79[88], _ = new(big.Int).SetString("1302275861726491392", 10)
	feeFactorLsh79[89], _ = new(big.Int).SetString("1433407450364798464", 10)
	feeFactorLsh79[90], _ = new(big.Int).SetString("1577743225646022400", 10)
	feeFactorLsh79[91], _ = new(big.Int).SetString("1736612772201237760", 10)
	feeFactorLsh79[92], _ = new(big.Int).SetString("1911479556084044800", 10)
	feeFactorLsh79[93], _ = new(big.Int).SetString("2103954405849466368", 10)
	feeFactorLsh79[94], _ = new(big.Int).SetString("2315810351098914816", 10)
	feeFactorLsh79[95], _ = new(big.Int).SetString("2548998955180110336", 10)
	feeFactorLsh79[96], _ = new(big.Int).SetString("2805668292494715904", 10)
	feeFactorLsh79[97], _ = new(big.Int).SetString("3088182736016075264", 10)
	feeFactorLsh79[98], _ = new(big.Int).SetString("3399144737294597120", 10)
	feeFactorLsh79[99], _ = new(big.Int).SetString("3741418799582781440", 10)
	feeFactorLsh79[100], _ = new(big.Int).SetString("4118157864914209280", 10)
	feeFactorLsh79[101], _ = new(big.Int).SetString("4532832358207517184", 10)
	feeFactorLsh79[102], _ = new(big.Int).SetString("4989262155942415360", 10)
	feeFactorLsh79[103], _ = new(big.Int).SetString("5491651773895053312", 10)
	feeFactorLsh79[104], _ = new(big.Int).SetString("6044629098073145344", 10)
	feeFactorLsh79[105], _ = new(big.Int).SetString("6653288015630603264", 10)
	feeFactorLsh79[106], _ = new(big.Int).SetString("7323235338466789376", 10)
	feeFactorLsh79[107], _ = new(big.Int).SetString("8060642451758603264", 10)
	feeFactorLsh79[108], _ = new(big.Int).SetString("8872302163198801920", 10)
	feeFactorLsh79[109], _ = new(big.Int).SetString("9765691276621297664", 10)
	feeFactorLsh79[110], _ = new(big.Int).SetString("10749039466425620480", 10)
	feeFactorLsh79[111], _ = new(big.Int).SetString("11831405087254624256", 10)
	feeFactorLsh79[112], _ = new(big.Int).SetString("13022758617264914432", 10)
	feeFactorLsh79[113], _ = new(big.Int).SetString("14334074503647985664", 10)
	feeFactorLsh79[114], _ = new(big.Int).SetString("15777432256460224512", 10)
	feeFactorLsh79[115], _ = new(big.Int).SetString("17366127722012377088", 10)
	feeFactorLsh79[116], _ = new(big.Int).SetString("19114795560840450048", 10)
	feeFactorLsh79[117], _ = new(big.Int).SetString("21039544058494664704", 10)
	feeFactorLsh79[118], _ = new(big.Int).SetString("23158103510989152256", 10)
	feeFactorLsh79[119], _ = new(big.Int).SetString("25489989551801102336", 10)
	feeFactorLsh79[120], _ = new(big.Int).SetString("28056682924947156992", 10)
	feeFactorLsh79[121], _ = new(big.Int).SetString("30881827360160751616", 10)
	feeFactorLsh79[122], _ = new(big.Int).SetString("33991447372945973248", 10)
	feeFactorLsh79[123], _ = new(big.Int).SetString("37414187995827814400", 10)
	feeFactorLsh79[124], _ = new(big.Int).SetString("41181578649142091776", 10)
	feeFactorLsh79[125], _ = new(big.Int).SetString("45328323582075174912", 10)
	feeFactorLsh79[126], _ = new(big.Int).SetString("49892621559424155648", 10)
	feeFactorLsh79[127], _ = new(big.Int).SetString("54916517738950524928", 10)
	feeFactorLsh79[128], _ = new(big.Int).SetString("60446290980731453440", 10)
	feeFactorLsh79[129], _ = new(big.Int).SetString("66532880156306030592", 10)
	feeFactorLsh79[130], _ = new(big.Int).SetString("73232353384667897856", 10)
	feeFactorLsh79[131], _ = new(big.Int).SetString("80606424517586026496", 10)
	feeFactorLsh79[132], _ = new(big.Int).SetString("88723021631988023296", 10)
	feeFactorLsh79[133], _ = new(big.Int).SetString("97656912766212980736", 10)
	feeFactorLsh79[134], _ = new(big.Int).SetString("107490394664256208896", 10)
	feeFactorLsh79[135], _ = new(big.Int).SetString("118314050872546246656", 10)
	feeFactorLsh79[136], _ = new(big.Int).SetString("130227586172649144320", 10)
	feeFactorLsh79[137], _ = new(big.Int).SetString("143340745036479856640", 10)
	feeFactorLsh79[138], _ = new(big.Int).SetString("157774322564602232832", 10)
	feeFactorLsh79[139], _ = new(big.Int).SetString("173661277220123770880", 10)
	feeFactorLsh79[140], _ = new(big.Int).SetString("191147955608404492288", 10)
	feeFactorLsh79[141], _ = new(big.Int).SetString("210395440584946647040", 10)
	feeFactorLsh79[142], _ = new(big.Int).SetString("231581035109891506176", 10)
	feeFactorLsh79[143], _ = new(big.Int).SetString("254899895518011031552", 10)
	feeFactorLsh79[144], _ = new(big.Int).SetString("280566829249471578112", 10)
	feeFactorLsh79[145], _ = new(big.Int).SetString("308818273601607499776", 10)
	feeFactorLsh79[146], _ = new(big.Int).SetString("339914473729459748864", 10)
	feeFactorLsh79[147], _ = new(big.Int).SetString("374141879958278111232", 10)
	feeFactorLsh79[148], _ = new(big.Int).SetString("411815786491420934144", 10)
	feeFactorLsh79[149], _ = new(big.Int).SetString("453283235820751683584", 10)
	feeFactorLsh79[150], _ = new(big.Int).SetString("498926215594241556480", 10)
	feeFactorLsh79[151], _ = new(big.Int).SetString("549165177389505314816", 10)
	feeFactorLsh79[152], _ = new(big.Int).SetString("604462909807314599936", 10)
	feeFactorLsh79[153], _ = new(big.Int).SetString("665328801563060338688", 10)
	feeFactorLsh79[154], _ = new(big.Int).SetString("732323533846678994944", 10)
	feeFactorLsh79[155], _ = new(big.Int).SetString("806064245175860330496", 10)
	feeFactorLsh79[156], _ = new(big.Int).SetString("887230216319880134656", 10)
	feeFactorLsh79[157], _ = new(big.Int).SetString("976569127662129774592", 10)
	feeFactorLsh79[158], _ = new(big.Int).SetString("1074903946642561957888", 10)
	feeFactorLsh79[159], _ = new(big.Int).SetString("1183140508725462433792", 10)
	feeFactorLsh79[160], _ = new(big.Int).SetString("1302275861726491312128", 10)
	feeFactorLsh79[161], _ = new(big.Int).SetString("1433407450364798566400", 10)
	feeFactorLsh79[162], _ = new(big.Int).SetString("1577743225646022393856", 10)
	feeFactorLsh79[163], _ = new(big.Int).SetString("1736612772201237577728", 10)
	feeFactorLsh79[164], _ = new(big.Int).SetString("1911479556084044922880", 10)
	feeFactorLsh79[165], _ = new(big.Int).SetString("2103954405849466404864", 10)
	feeFactorLsh79[166], _ = new(big.Int).SetString("2315810351098914930688", 10)
	feeFactorLsh79[167], _ = new(big.Int).SetString("2548998955180110643200", 10)
	feeFactorLsh79[168], _ = new(big.Int).SetString("2805668292494716043264", 10)
	feeFactorLsh79[169], _ = new(big.Int).SetString("3088182736016075390976", 10)
	feeFactorLsh79[170], _ = new(big.Int).SetString("3399144737294597488640", 10)
	feeFactorLsh79[171], _ = new(big.Int).SetString("3741418799582781374464", 10)
	feeFactorLsh79[172], _ = new(big.Int).SetString("4118157864914209210368", 10)
	feeFactorLsh79[173], _ = new(big.Int).SetString("4532832358207517097984", 10)
	feeFactorLsh79[174], _ = new(big.Int).SetString("4989262155942415302656", 10)
	feeFactorLsh79[175], _ = new(big.Int).SetString("5491651773895053148160", 10)
	feeFactorLsh79[176], _ = new(big.Int).SetString("6044629098073145999360", 10)
	feeFactorLsh79[177], _ = new(big.Int).SetString("6653288015630603124736", 10)
	feeFactorLsh79[178], _ = new(big.Int).SetString("7323235338466790211584", 10)
	feeFactorLsh79[179], _ = new(big.Int).SetString("8060642451758603304960", 10)
	feeFactorLsh79[180], _ = new(big.Int).SetString("8872302163198802395136", 10)
	feeFactorLsh79[181], _ = new(big.Int).SetString("9765691276621298794496", 10)
	feeFactorLsh79[182], _ = new(big.Int).SetString("10749039466425620627456", 10)
	feeFactorLsh79[183], _ = new(big.Int).SetString("11831405087254623813632", 10)
	feeFactorLsh79[184], _ = new(big.Int).SetString("13022758617264913645568", 10)
	feeFactorLsh79[185], _ = new(big.Int).SetString("14334074503647983566848", 10)
	feeFactorLsh79[186], _ = new(big.Int).SetString("15777432256460225511424", 10)
	feeFactorLsh79[187], _ = new(big.Int).SetString("17366127722012377874432", 10)
	feeFactorLsh79[188], _ = new(big.Int).SetString("19114795560840447655936", 10)
	feeFactorLsh79[189], _ = new(big.Int).SetString("21039544058494662475776", 10)
	feeFactorLsh79[190], _ = new(big.Int).SetString("23158103510989150355456", 10)
	feeFactorLsh79[191], _ = new(big.Int).SetString("25489989551801103286272", 10)
	feeFactorLsh79[192], _ = new(big.Int).SetString("28056682924947158335488", 10)
	feeFactorLsh79[193], _ = new(big.Int).SetString("30881827360160754958336", 10)
	feeFactorLsh79[194], _ = new(big.Int).SetString("33991447372945971740672", 10)
	feeFactorLsh79[195], _ = new(big.Int).SetString("37414187995827814793216", 10)
	feeFactorLsh79[196], _ = new(big.Int).SetString("41181578649142095249408", 10)
	feeFactorLsh79[197], _ = new(big.Int).SetString("45328323582075170979840", 10)
	feeFactorLsh79[198], _ = new(big.Int).SetString("49892621559424153026560", 10)
	feeFactorLsh79[199], _ = new(big.Int).SetString("54916517738950525190144", 10)
	feeFactorLsh79[200], _ = new(big.Int).SetString("60446290980731462090752", 10)
	feeFactorLsh79[201], _ = new(big.Int).SetString("66532880156306033344512", 10)
	feeFactorLsh79[202], _ = new(big.Int).SetString("73232353384667900018688", 10)
	feeFactorLsh79[203], _ = new(big.Int).SetString("80606424517586030952448", 10)
	feeFactorLsh79[204], _ = new(big.Int).SetString("88723021631988017659904", 10)
	feeFactorLsh79[205], _ = new(big.Int).SetString("97656912766212987944960", 10)
	feeFactorLsh79[206], _ = new(big.Int).SetString("107490394664256189497344", 10)
	feeFactorLsh79[207], _ = new(big.Int).SetString("118314050872546242330624", 10)
	feeFactorLsh79[208], _ = new(big.Int).SetString("130227586172649140649984", 10)
	feeFactorLsh79[209], _ = new(big.Int).SetString("143340745036479844057088", 10)
	feeFactorLsh79[210], _ = new(big.Int).SetString("157774322564602238337024", 10)
	feeFactorLsh79[211], _ = new(big.Int).SetString("173661277220123782938624", 10)
	feeFactorLsh79[212], _ = new(big.Int).SetString("191147955608404493336576", 10)
	feeFactorLsh79[213], _ = new(big.Int).SetString("210395440584946633146368", 10)
	feeFactorLsh79[214], _ = new(big.Int).SetString("231581035109891511943168", 10)
	feeFactorLsh79[215], _ = new(big.Int).SetString("254899895518011024474112", 10)
	feeFactorLsh79[216], _ = new(big.Int).SetString("280566829249471583354880", 10)
	feeFactorLsh79[217], _ = new(big.Int).SetString("308818273601607516028928", 10)
	feeFactorLsh79[218], _ = new(big.Int).SetString("339914473729459734183936", 10)
	feeFactorLsh79[219], _ = new(big.Int).SetString("374141879958278114377728", 10)
	feeFactorLsh79[220], _ = new(big.Int).SetString("411815786491420902162432", 10)
	feeFactorLsh79[221], _ = new(big.Int).SetString("453283235820751760130048", 10)
	feeFactorLsh79[222], _ = new(big.Int).SetString("498926215594241513488384", 10)
	feeFactorLsh79[223], _ = new(big.Int).SetString("549165177389505251901440", 10)
	feeFactorLsh79[224], _ = new(big.Int).SetString("604462909807314587353088", 10)
	feeFactorLsh79[225], _ = new(big.Int).SetString("6044629098073145873530880", 10)
	feeFactorLsh79[226], _ = new(big.Int).SetString("60446290980731458735308800", 10)
	feeFactorLsh79[227], _ = new(big.Int).SetString("604462909807314587353088000", 10)
	feeFactorLsh79[228], _ = new(big.Int).SetString("6044629098073145873530880000", 10)
	feeFactorLsh79[229], _ = new(big.Int).SetString("60446290980731458735308800000", 10)
	feeFactorLsh79[230], _ = new(big.Int).SetString("604462909807314587353088000000", 10)
	feeFactorLsh79[231], _ = new(big.Int).SetString("6044629098073145873530880000000", 10)
	feeFactorLsh79[232], _ = new(big.Int).SetString("60446290980731458735308800000000", 10)
	feeFactorLsh79[233], _ = new(big.Int).SetString("604462909807314587353088000000000", 10)
	feeFactorLsh79[234], _ = new(big.Int).SetString("6044629098073145873530880000000000", 10)
	feeFactorLsh79[235], _ = new(big.Int).SetString("60446290980731458735308800000000000", 10)
	feeFactorLsh79[236], _ = new(big.Int).SetString("604462909807314587353088000000000000", 10)
	feeFactorLsh79[237], _ = new(big.Int).SetString("6044629098073145873530880000000000000", 10)
	feeFactorLsh79[238], _ = new(big.Int).SetString("60446290980731458735308800000000000000", 10)
	feeFactorLsh79[239], _ = new(big.Int).SetString("604462909807314587353088000000000000000", 10)
	feeFactorLsh79[240], _ = new(big.Int).SetString("6044629098073145873530880000000000000000", 10)
	feeFactorLsh79[241], _ = new(big.Int).SetString("60446290980731458735308800000000000000000", 10)
	feeFactorLsh79[242], _ = new(big.Int).SetString("604462909807314587353088000000000000000000", 10)
	feeFactorLsh79[243], _ = new(big.Int).SetString("6044629098073145873530880000000000000000000", 10)
	feeFactorLsh79[244], _ = new(big.Int).SetString("60446290980731458735308800000000000000000000", 10)
	feeFactorLsh79[245], _ = new(big.Int).SetString("604462909807314587353088000000000000000000000", 10)
	feeFactorLsh79[246], _ = new(big.Int).SetString("6044629098073145873530880000000000000000000000", 10)
	feeFactorLsh79[247], _ = new(big.Int).SetString("60446290980731458735308800000000000000000000000", 10)
	feeFactorLsh79[248], _ = new(big.Int).SetString("604462909807314587353088000000000000000000000000", 10)
	feeFactorLsh79[249], _ = new(big.Int).SetString("6044629098073145873530880000000000000000000000000", 10)
	feeFactorLsh79[250], _ = new(big.Int).SetString("60446290980731458735308800000000000000000000000000", 10)
	feeFactorLsh79[251], _ = new(big.Int).SetString("604462909807314587353088000000000000000000000000000", 10)
	feeFactorLsh79[252], _ = new(big.Int).SetString("6044629098073145873530880000000000000000000000000000", 10)
	feeFactorLsh79[253], _ = new(big.Int).SetString("60446290980731458735308800000000000000000000000000000", 10)
	feeFactorLsh79[254], _ = new(big.Int).SetString("604462909807314587353088000000000000000000000000000000", 10)
	feeFactorLsh79[255], _ = new(big.Int).SetString("6044629098073145873530880000000000000000000000000000000", 10)
}
