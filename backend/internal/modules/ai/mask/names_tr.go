package mask

import "strings"

// firstNames are common Turkish first names (folded with foldName). A first
// name followed by a capitalized word is masked as a person's name (#169).
// Names that are also everyday words in documents (e.g. "Deniz", "Umut",
// "Can") are included on purpose: masking errs on the side of hiding, and
// only a following capitalized word triggers it.
var firstNames = func() map[string]bool {
	const list = `
Abdullah Adem Adnan Ahmet Akın Ali Alp Alper Altan Arda Arif Arslan Aslı Atakan Ayberk Aydın Aygül Ayhan
Aylin Ayşe Ayşegül Aysel Aysun Aytaç Azize Bahar Banu Barış Başak Batuhan Bayram Bekir Belgin Belma Berk
Berkay Berna Betül Beyza Bilal Bilge Birsen Bora Buğra Bülent Burak Burcu Burhan Büşra Cahit Can Canan Cem
Cemal Cemil Cengiz Cenk Ceren Cihan Coşkun Cüneyt Çağlar Çağrı Çiğdem Damla Demet Deniz Derya Devrim Dilek
Dilara Doğan Duygu Ebru Ece Eda Elif Emel Emine Emir Emre Ender Engin Enes Erdal Erdem Erhan Erkan Ersin
Ertan Ertuğrul Esra Esin Evren Eylül Ezgi Fadime Fahri Fatih Fatma Ferhat Feride Feyza Figen Filiz Fikret
Fuat Funda Furkan Gamze Gizem Gökay Gökhan Gönül Gül Gülay Gülsüm Gülşen Günay Güler Gürkan Hakan Halil
Hale Halit Hamza Handan Hande Harun Hasan Hatice Havva Hayri Hilal Hülya Hüseyin İbrahim İlhan İlker İlknur
İpek İrem İsmail İsmet Kadir Kamil Kemal Kenan Kerem Kevser Koray Korkut Kübra Kürşat Leyla Mahmut Mehmet
Melek Melike Melis Meltem Merve Mert Mesut Metin Mine Muhammed Murat Musa Mustafa Mutlu Müge Necati Nail
Nazlı Nebahat Necla Nergis Nesrin Neslihan Nevin Nihat Nilay Nilgün Nur Nuran Nurcan Nuri Oğuz Oğuzhan Okan
Onur Orhan Osman Ozan Ömer Önder Özge Özgür Özkan Pelin Pınar Ramazan Rabia Recep Reşat Rıza Rukiye Sabri
Sadık Salih Sami Sedat Sefa Selahattin Selim Selin Selma Semih Semra Serap Serdar Serkan Sevda Sevgi Sevim
Seda Sena Serpil Sibel Sinan Sinem Songül Suat Sultan Süleyman Şahin Şebnem Şenol Şerife Şevket Şeyma Şule
Şükrü Taha Tahsin Talha Tamer Tarık Tayfun Tolga Tuba Tuğba Tuğçe Tülay Tuncay Turgay Turan Uğur Ufuk Ümit
Umut Ünal Vedat Veli Volkan Yakup Yasemin Yasin Yavuz Yaşar Yeliz Yeşim Yıldız Yılmaz Yunus Yusuf Zafer
Zehra Zekeriya Zeki Zeynep Zübeyde Zülfikar`
	out := map[string]bool{}
	for _, n := range strings.Fields(list) {
		out[foldName(n)] = true
	}
	return out
}()
